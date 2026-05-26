I’ll fix the xAI OAuth sanitizer issue by updating `internal/runtime/executor/xai_oauth_executor.go` and adding focused coverage:

1. Update `filterXAIOAuthToolChoice` so scalar `tool_choice` values are handled after unsupported tools are stripped.
2. If the top-level `tools` array no longer exists and scalar `tool_choice` is `"required"` or another tool-requiring value, delete `tool_choice` to avoid forwarding an unsatisfiable request.
3. Preserve safe scalar values such as `"auto"` and `"none"` where appropriate.
4. Add a test in `internal/runtime/executor/xai_oauth_executor_test.go` for `tools` containing only unsupported custom tools with scalar `tool_choice: "required"`, asserting both `tools` and `tool_choice` are removed.
5. Run `gofmt` on touched Go files and verify with targeted tests, likely `go test ./internal/runtime/executor`.