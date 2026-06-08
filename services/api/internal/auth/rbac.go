package auth

const (
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

type Permission string

const (
	PermAgentWrite     Permission = "agent:write"
	PermAgentRead      Permission = "agent:read"
	PermToolWrite      Permission = "tool:write"
	PermToolRead       Permission = "tool:read"
	PermRunStart       Permission = "run:start"
	PermRunRead        Permission = "run:read"
	PermRunApprove     Permission = "run:approve"
	PermMemoryWrite    Permission = "memory:write"
	PermMemoryRead     Permission = "memory:read"
	PermConnectorWrite Permission = "connector:write"
	PermConnectorRead  Permission = "connector:read"
	PermAdminAll       Permission = "admin:*"
)

var rolePermissions = map[string][]Permission{
	RoleAdmin:  {PermAgentWrite, PermAgentRead, PermToolWrite, PermToolRead, PermRunStart, PermRunRead, PermRunApprove, PermMemoryWrite, PermMemoryRead, PermConnectorWrite, PermConnectorRead, PermAdminAll},
	RoleMember: {PermAgentWrite, PermAgentRead, PermToolRead, PermRunStart, PermRunRead, PermRunApprove, PermMemoryWrite, PermMemoryRead, PermConnectorRead},
	RoleViewer: {PermAgentRead, PermToolRead, PermRunRead, PermMemoryRead, PermConnectorRead},
}

func HasPermission(role string, perm Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm || p == PermAdminAll {
			return true
		}
	}
	return false
}
