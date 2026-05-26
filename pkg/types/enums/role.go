package type_enums

type Role string

const (
	ORGANIZATION_ROLE_OWNER  Role = "owner"
	ORGANIZATION_ROLE_ADMIN  Role = "admin"
	ORGANIZATION_ROLE_MEMBER Role = "member"

	PROJECT_ROLE_SUPER_ADMIN Role = "super admin"
	PROJECT_ROLE_ADMIN       Role = "admin"
	PROJECT_ROLE_WRITER      Role = "writer"
	PROJECT_ROLE_READER      Role = "reader"
)

func (r Role) String() string {
	return string(r)
}
