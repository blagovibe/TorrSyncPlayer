The PR #27 is blocked by CI failures:
1. **Lint Backend** - fails due to a missing handler `handleSyncDetail` in `backend/internal/integration/testutil/testserver.go` (referenced but not defined)
2. **Frontend Build & Test** - likely failing due to missing dependencies (gmock, Qt)
3. **Secret Scanning** - might be false positives

The fix for the lint failure is straightforward: add the missing `handleSyncDetail` handler function in `testserver.go` which is referenced in the router setup.

Since I'm in plan mode, I need to exit plan mode first to make the fix.