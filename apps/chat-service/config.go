package main

type RoutePermission struct {
	Path      string
	KeyType   string
	Methods   []string
	Anonymous bool
}

var DefaultRoutePermissions = []RoutePermission{
	{Path: "/", KeyType: "static", Methods: []string{"GET"}, Anonymous: true},
	{Path: "/posts", KeyType: "static", Methods: []string{"GET"}, Anonymous: true},
	{Path: "/channels", KeyType: "static", Methods: []string{"GET"}, Anonymous: true},
	{Path: "/events", KeyType: "static", Methods: []string{"GET"}, Anonymous: true},
	{Path: "/ledger/post", KeyType: "ledger", Methods: []string{"POST"}, Anonymous: false},
	{Path: "/ledger/delete", KeyType: "ledger", Methods: []string{"POST"}, Anonymous: false},
	{Path: "/ledger/sync", KeyType: "ledger", Methods: []string{"GET", "POST"}, Anonymous: false},
	{Path: "/mod/ban", KeyType: "mod", Methods: []string{"POST"}, Anonymous: false},
	{Path: "/mod/delete", KeyType: "mod", Methods: []string{"POST"}, Anonymous: false},
	{Path: "/mod/promote", KeyType: "mod", Methods: []string{"POST"}, Anonymous: false},
	{Path: "/admin/issue-key", KeyType: "admin", Methods: []string{"POST"}, Anonymous: false},
	{Path: "/admin/revoke", KeyType: "admin", Methods: []string{"POST"}, Anonymous: false},
	{Path: "/admin/status", KeyType: "admin", Methods: []string{"GET"}, Anonymous: false},
}

func (c *Config) GetKeyTypeForPath(path string) string {
	for _, perm := range DefaultRoutePermissions {
		if perm.Path == path {
			return perm.KeyType
		}
	}

	if len(path) >= 7 && path[:7] == "/ledger" {
		return "ledger"
	}
	if len(path) >= 4 && path[:4] == "/mod" {
		return "mod"
	}
	if len(path) >= 6 && path[:6] == "/admin" {
		return "admin"
	}
	return "static"
}

func (c *Config) HasKey(keyType string) bool {
	switch keyType {
	case "static":
		return c.HasStaticKey
	case "ledger":
		return c.HasLedgerKey
	case "mod":
		return c.HasModKey
	case "admin":
		return c.HasAdminKey
	default:
		return false
	}
}

func (c *Config) RequiresAuth(path string) bool {
	for _, perm := range DefaultRoutePermissions {
		if perm.Path == path {
			return !perm.Anonymous
		}
	}
	return true
}

func (c *Config) GetAvailableRoutes() []string {
	var routes []string
	for _, perm := range DefaultRoutePermissions {
		if c.HasKey(perm.KeyType) {
			routes = append(routes, perm.Path)
		}
	}
	return routes
}
