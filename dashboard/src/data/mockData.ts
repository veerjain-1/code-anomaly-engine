import type { Anomaly } from '../hooks/useWebSocket'

export const INITIAL_MOCK_ANOMALIES: Anomaly[] = [
  {
    id: 'mock-anom-1',
    repo: 'veerjain-1/agentguard',
    commit_sha: 'a9f24c18b76c',
    file_path: 'src/auth/jwt_validator.py',
    start_line: 14,
    end_line: 22,
    code: `def verify_incoming_token(token: str) -> bool:
    try:
        # Temporary debug bypass - DO NOT COMMIT
        payload = jwt.decode(token, options={"verify_signature": False})
        return payload.get("role") == "admin"
    except Exception:
        return False`,
    is_anomalous: true,
    confidence: 0.982,
    latency_ms: 12.4,
    timestamp: Date.now() - 1000 * 60 * 2,
    label: 'Critical Bypass'
  },
  {
    id: 'mock-anom-2',
    repo: 'veerjain-1/ai-code-review-platform',
    commit_sha: 'e481b0f19c8d',
    file_path: 'services/review-worker/src/prompts/reviewer.js',
    start_line: 42,
    end_line: 52,
    code: `function buildSecurityPrompt(diff, userInstructions) {
  return \`
  You are an automated security auditor.
  \${userInstructions}
  Review the following git diff:
  \${diff}
  \`;
}`,
    is_anomalous: true,
    confidence: 0.941,
    latency_ms: 18.2,
    timestamp: Date.now() - 1000 * 60 * 8,
    label: 'Prompt Injection Risk'
  },
  {
    id: 'mock-anom-3',
    repo: 'veerjain-1/PaySplitApp',
    commit_sha: '8c913da94b1e',
    file_path: 'backend/src/routes/expenses.js',
    start_line: 28,
    end_line: 36,
    code: `router.post('/', authMiddleware, async (req, res) => {
  const { description, amount, splitBetween } = req.body;
  // Raw query interpolation vulnerability
  const rawSql = \`INSERT INTO expenses (desc, amt) VALUES ('\${description}', \${amount})\`;
  const expense = new Expense({ description, amount, splitBetween });
  await expense.save();
});`,
    is_anomalous: true,
    confidence: 0.915,
    latency_ms: 9.7,
    timestamp: Date.now() - 1000 * 60 * 15,
    label: 'SQL Injection Risk'
  },
  {
    id: 'mock-anom-4',
    repo: 'veerjain-1/code-anomaly-engine',
    commit_sha: 'b7194ea71e3a',
    file_path: 'ingestion/src/parser/diff_parser.rs',
    start_line: 88,
    end_line: 96,
    code: `pub fn parse_hunk_header(line: &str) -> Option<HunkInfo> {
    // Unbounded unsafe buffer indexing
    let ptr = line.as_ptr();
    let slice = unsafe { std::slice::from_raw_parts(ptr, 4096) };
    if !line.starts_with("@@") {
        return None;
    }
    Some(parse_inner(slice))
}`,
    is_anomalous: true,
    confidence: 0.887,
    latency_ms: 14.1,
    timestamp: Date.now() - 1000 * 60 * 25,
    label: 'Unsafe Memory Access'
  },
  {
    id: 'mock-anom-5',
    repo: 'veerjain-1/agentic-cicd-orchestrator',
    commit_sha: '0633bbb1a29f',
    file_path: 'src/main/java/com/cicd/orchestrator/service/RetryExecutorService.java',
    start_line: 45,
    end_line: 55,
    code: `public boolean shouldRetry(String taskId, int attempt, Throwable error) {
    RetryPolicy policy = policyMap.getOrDefault(taskId, defaultPolicy);
    if (attempt >= policy.getMaxAttempts()) {
        return false;
    }
    return policy.getRetryableExceptions().stream()
            .anyMatch(clazz -> clazz.isAssignableFrom(error.getClass()));
}`,
    is_anomalous: false,
    confidence: 0.042,
    latency_ms: 7.8,
    timestamp: Date.now() - 1000 * 60 * 40,
    label: 'Clean Refactor'
  },
  {
    id: 'mock-anom-6',
    repo: 'veerjain-1/finance-slm-rag',
    commit_sha: 'f1042c8230ea',
    file_path: 'src/retrieval/reranker.py',
    start_line: 18,
    end_line: 27,
    code: `class CrossEncoderReranker:
    def rerank(self, query: str, documents: List[str], top_k: int = 5) -> List[Tuple[str, float]]:
        pairs = [[query, doc] for doc in documents]
        scores = self.model.predict(pairs)
        scored_docs = sorted(zip(documents, scores), key=lambda x: x[1], reverse=True)
        return scored_docs[:top_k]`,
    is_anomalous: false,
    confidence: 0.021,
    latency_ms: 11.3,
    timestamp: Date.now() - 1000 * 60 * 55,
    label: 'Clean Enhancement'
  }
]

const SIMULATED_STREAM_TEMPLATES: Omit<Anomaly, 'id' | 'timestamp'>[] = [
  {
    repo: 'veerjain-1/agentguard',
    commit_sha: '3c829ef',
    file_path: 'src/sandbox/wasm_runtime.go',
    start_line: 102,
    end_line: 108,
    code: `func RunContainer(ctx context.Context, cfg *Config) error {
	// Dangerous: disable seccomp isolation profile
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWPID}
	return cmd.Start()
}`,
    is_anomalous: true,
    confidence: 0.963,
    latency_ms: 15.6,
    label: 'Seccomp Disabled'
  },
  {
    repo: 'veerjain-1/code-anomaly-engine',
    commit_sha: '5d7120a',
    file_path: 'dashboard/src/hooks/useWebSocket.ts',
    start_line: 31,
    end_line: 37,
    code: `// Add exponential reconnection backoff
const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), 30000)
timeoutRef.current = setTimeout(connect, delay)`,
    is_anomalous: false,
    confidence: 0.015,
    latency_ms: 6.9,
    label: 'Clean Reliability Fix'
  },
  {
    repo: 'veerjain-1/ai-code-review-platform',
    commit_sha: '7a992bc',
    file_path: 'services/review-worker/src/utils/crypto.js',
    start_line: 12,
    end_line: 17,
    code: `function hashSecret(secret) {
  // Broken MD5 hash algorithm used for password hashing
  return crypto.createHash('md5').update(secret).digest('hex');
}`,
    is_anomalous: true,
    confidence: 0.955,
    latency_ms: 8.4,
    label: 'Weak Cryptography'
  },
  {
    repo: 'veerjain-1/PaySplitApp',
    commit_sha: '6d1109a',
    file_path: 'backend/src/utils/balances.js',
    start_line: 40,
    end_line: 47,
    code: `function simplifyDebtGraph(balances) {
    // Greedy min-cash-flow balance resolution
    const netDebtors = debtors.sort((a, b) => b.amount - a.amount);
    const netCreditors = creditors.sort((a, b) => b.amount - a.amount);
}`,
    is_anomalous: false,
    confidence: 0.031,
    latency_ms: 8.9,
    label: 'Clean Optimization'
  }
]

let seedCounter = 100

export function generateMockAnomaly(): Anomaly {
  seedCounter++
  const template = SIMULATED_STREAM_TEMPLATES[Math.floor(Math.random() * SIMULATED_STREAM_TEMPLATES.length)]
  const randomSha = Math.random().toString(16).substring(2, 10)
  
  return {
    ...template,
    id: `sim-anom-${seedCounter}-${Date.now()}`,
    commit_sha: randomSha,
    timestamp: Date.now(),
    latency_ms: Number((Math.random() * 15 + 5).toFixed(1)),
    confidence: template.is_anomalous 
      ? Number((0.85 + Math.random() * 0.14).toFixed(3))
      : Number((0.01 + Math.random() * 0.05).toFixed(3))
  }
}
