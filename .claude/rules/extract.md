---
paths:
  - "internal/extract/**"
  - "internal/pipeline/**"
  - "testdata/**"
---

# Extraction and pipeline rules

- The `Extractor` interface in `internal/extract` is the only boundary. Code outside
  `internal/extract/*` must not import a provider package or branch on provider name.
- Every extractor's output goes through the shared validator: referenced IDs exist, enum values are valid,
  at most 2 questions, text spans fall inside the entry body. Invalid output is a failed run, never partial state.
- Extractors never write to the database. They return an `ExtractionResult`; the pipeline stores it.
- The mock extractor must stay deterministic: same entry + snapshot in, same result out.
  Tests depend on it. Any randomness or latency is opt-in via config.
- Any change to a prompt template bumps `PromptVersion`. Prompts live in files, not inline string literals.
- Every run records provider, model, prompt version, input snapshot, raw output, status, and latency.
- A change to extraction behavior comes with a new or updated fixture in `testdata/entries/`.
- Failure handling: timeouts on every LLM call; one retry that feeds validation errors back to the model;
  then mark the run failed and emit `run.failed`. The entry itself is always already saved.
- Never log entry text or model output at info level. Debug level only, and only when explicitly enabled.
- No test may call a real LLM. Real-provider checks go through `waypoint eval`, run manually.
