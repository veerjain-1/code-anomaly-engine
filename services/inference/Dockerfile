FROM rust:1.78-slim-bookworm AS builder
RUN apt-get update && apt-get install -y protobuf-compiler libssl-dev pkg-config && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY proto/ /app/proto/
COPY services/inference/ /app/
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates libssl3 && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/target/release/inference-server /usr/local/bin/inference-server
COPY services/inference/models/ /app/models/
WORKDIR /app
EXPOSE 50051
ENTRYPOINT ["inference-server"]
