//! Plain-HTTP `/health` endpoint.
//!
//! The inference engine's primary API is gRPC (see `InferenceService::health_check`
//! for the richer, model-aware health RPC). This module adds a minimal
//! standalone HTTP listener for infra that can't speak gRPC health-checking --
//! load balancers, Docker `HEALTHCHECK`, or a plain `curl` liveness probe.
//! It intentionally does not depend on the engine/model state: it answers as
//! soon as the process is up, which is the right semantics for a liveness
//! (not readiness) check.

use http_body_util::Full;
use hyper::body::Bytes;
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{Request, Response, StatusCode};
use hyper_util::rt::TokioIo;
use std::convert::Infallible;
use std::net::SocketAddr;
use tokio::net::TcpListener;
use tracing::{error, info, warn};

async fn handle(
    req: Request<hyper::body::Incoming>,
) -> Result<Response<Full<Bytes>>, Infallible> {
    let response = if req.uri().path() == "/health" {
        Response::builder()
            .status(StatusCode::OK)
            .header("content-type", "application/json")
            .body(Full::new(Bytes::from(r#"{"status":"ok"}"#)))
            .unwrap()
    } else {
        Response::builder()
            .status(StatusCode::NOT_FOUND)
            .body(Full::new(Bytes::new()))
            .unwrap()
    };

    Ok(response)
}

/// Serve the `/health` endpoint on `addr` until the process exits.
///
/// Runs forever; intended to be spawned as a background task
/// (`tokio::spawn(health::serve(addr))`) alongside the main gRPC server.
pub async fn serve(addr: SocketAddr) {
    let listener = match TcpListener::bind(addr).await {
        Ok(listener) => listener,
        Err(e) => {
            error!(%addr, error = %e, "Failed to bind health check listener");
            return;
        }
    };

    info!(%addr, "Health check HTTP server listening on /health");

    loop {
        let (stream, _) = match listener.accept().await {
            Ok(conn) => conn,
            Err(e) => {
                warn!(error = %e, "Failed to accept health check connection");
                continue;
            }
        };

        let io = TokioIo::new(stream);

        tokio::spawn(async move {
            if let Err(e) = http1::Builder::new()
                .serve_connection(io, service_fn(handle))
                .await
            {
                warn!(error = %e, "Error serving health check connection");
            }
        });
    }
}
