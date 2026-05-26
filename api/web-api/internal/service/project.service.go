package internal_service

import (
	"context"

	internal_entity "github.com/rapidaai/api/web-api/internal/entity"
	"github.com/rapidaai/pkg/types"
	web_api "github.com/rapidaai/protos"
)

type ProjectService interface {
	Create(ctx context.Context, auth types.Principle, organizationId uint64, name string, description string) (*internal_entity.Project, error)
	Update(ctx context.Context, auth types.Principle, projectId uint64, name *string, description *string) (*internal_entity.Project, error)
	Get(ctx context.Context, auth types.SimplePrinciple, projectId uint64) (*internal_entity.Project, error)
	GetAll(ctx context.Context, auth types.SimplePrinciple, organizationId uint64, criteria []*web_api.Criteria, paginate *web_api.Paginate) (int64, []*internal_entity.Project, error)
	GetAllByOrganization(ctx context.Context, auth types.SimplePrinciple, organizationId uint64, projectIds []uint64) ([]*internal_entity.Project, error)
	Archive(ctx context.Context, auth types.Principle, projectId uint64) (*internal_entity.Project, error)

	CreateCredential(ctx context.Context, auth types.Principle, name string, projectId, organizationId uint64) (*internal_entity.ProjectCredential, error)
	ArchiveCredential(ctx context.Context, auth types.Principle, credentialId, projectId, organizationId uint64) (*internal_entity.ProjectCredential, error)
	GetAllCredential(ctx context.Context, auth types.Principle, projectId, organizationId uint64, criteria []*web_api.Criteria, paginate *web_api.Paginate) (int64, []*internal_entity.ProjectCredential, error)
}
