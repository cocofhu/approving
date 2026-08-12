package runtime

import "github.com/cocofhu/approving/internal/mcp"

// newACPProvider builds a Cursor-backend ACP provider for tests.
// Production wiring uses NewProviderRegistry → newBaseACPProvider.
func newACPProvider(host *mcp.Host, opts Options) ExecProvider {
	return newBaseACPProvider(host, opts, BackendCursor)
}
