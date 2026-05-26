I’ll fix the README issue by updating the Z.AI file-backed auth example to match the actual loader contract:

1. Edit `README.md` in the Z.AI auth-file section.
2. Move `prefix` and `disable-cooling` from the nested `metadata` object to top-level JSON fields.
3. Replace the unsupported top-level-style `"header:X-Custom-Header"` example with the supported top-level `"headers"` object:
   ```json
   "headers": {
     "X-Custom-Header": "example-value"
   }
   ```
4. Remove the misleading nested `metadata` wrapper from the example.
5. Review the changed section and run a lightweight verification (`git diff`, plus no build needed since this is docs-only) unless you want a full test/build run.