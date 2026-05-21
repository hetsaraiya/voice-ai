package web_api

import (
	"context"
	"errors"
	"testing"

	web_config "github.com/rapidaai/api/web-api/config"
	internal_entity "github.com/rapidaai/api/web-api/internal/entity"
	internal_project_service "github.com/rapidaai/api/web-api/internal/service/project"
	internal_user_service "github.com/rapidaai/api/web-api/internal/service/user"
	app_config "github.com/rapidaai/config"
	external_clients "github.com/rapidaai/pkg/clients/external"
	external_emailer_template "github.com/rapidaai/pkg/clients/external/emailer/template"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	gorm_models "github.com/rapidaai/pkg/models/gorm"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testPostgresConnector struct {
	db *gorm.DB
}

func (t *testPostgresConnector) Connect(ctx context.Context) error {
	return nil
}

func (t *testPostgresConnector) Name() string {
	return "test-postgres"
}

func (t *testPostgresConnector) IsConnected(ctx context.Context) bool {
	return true
}

func (t *testPostgresConnector) Disconnect(ctx context.Context) error {
	return nil
}

func (t *testPostgresConnector) Query(ctx context.Context, qry string, dest interface{}) error {
	return t.DB(ctx).Raw(qry).Scan(dest).Error
}

func (t *testPostgresConnector) DB(ctx context.Context) *gorm.DB {
	if tx, ok := connectors.PostgresTxFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return t.db.WithContext(ctx)
}

type testEmailer struct {
	err   error
	calls int
	to    external_clients.Contact
	args  map[string]string
}

func (t *testEmailer) EmailText(ctx context.Context, to external_clients.Contact, subject string, content string) error {
	return nil
}

func (t *testEmailer) EmailRichText(ctx context.Context, to external_clients.Contact, subject string, template external_emailer_template.TemplateName, args map[string]string) error {
	t.calls++
	t.to = to
	t.args = args
	return t.err
}

func (t *testEmailer) EmailTemplate(ctx context.Context, to external_clients.Contact, subject string, templateId string, args map[string]string) error {
	return nil
}

func newProjectAPITest(t *testing.T) (*webProjectGRPCApi, *gorm.DB, *testEmailer) {
	t.Helper()
	logger, err := commons.NewApplicationLogger()
	require.NoError(t, err)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger2Discard()})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE organizations (id integer primary key, created_date datetime, updated_date datetime, status text, created_by integer, updated_by integer, name text, description text, size text, industry text, contact text)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE projects (id integer primary key, created_date datetime, updated_date datetime, status text, created_by integer, updated_by integer, organization_id integer, name text, description text)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE user_auths (id integer primary key, created_date datetime, updated_date datetime, status text, created_by integer, updated_by integer, name text, email text, password text, source text)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE user_auth_tokens (id integer primary key, created_date datetime, updated_date datetime, status text, created_by integer, updated_by integer, user_auth_id integer, token_type text, token text, expire_at datetime)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE user_organization_roles (id integer primary key, created_date datetime, updated_date datetime, status text, created_by integer, updated_by integer, user_auth_id integer, organization_id integer, role text, UNIQUE (user_auth_id, organization_id, status))`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE user_project_roles (id integer primary key, created_date datetime, updated_date datetime, status text, created_by integer, updated_by integer, user_auth_id integer, project_id integer, role text, UNIQUE (user_auth_id, project_id, status))`).Error)
	require.NoError(t, db.Create(&internal_entity.Organization{
		Audited: gorm_models.Audited{Id: 10},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ACTIVE,
			CreatedBy: 1,
		},
		Name:        "Acme",
		Description: "Acme org",
		Industry:    "software",
		Contact:     "admin@example.com",
	}).Error)
	require.NoError(t, db.Create(&internal_entity.Organization{
		Audited: gorm_models.Audited{Id: 20},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ACTIVE,
			CreatedBy: 1,
		},
		Name:        "Other",
		Description: "Other org",
		Industry:    "software",
		Contact:     "admin@example.com",
	}).Error)
	require.NoError(t, db.Create(&internal_entity.Project{
		Audited: gorm_models.Audited{Id: 100},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ACTIVE,
			CreatedBy: 1,
		},
		OrganizationId: 10,
		Name:           "Alpha",
		Description:    "Alpha project",
	}).Error)
	require.NoError(t, db.Create(&internal_entity.Project{
		Audited: gorm_models.Audited{Id: 101},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ACTIVE,
			CreatedBy: 1,
		},
		OrganizationId: 10,
		Name:           "Beta",
		Description:    "Beta project",
	}).Error)
	require.NoError(t, db.Create(&internal_entity.Project{
		Audited: gorm_models.Audited{Id: 200},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ACTIVE,
			CreatedBy: 1,
		},
		OrganizationId: 20,
		Name:           "Cross",
		Description:    "Cross project",
	}).Error)
	require.NoError(t, db.Create(&internal_entity.Project{
		Audited: gorm_models.Audited{Id: 300},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ARCHIEVE,
			CreatedBy: 1,
		},
		OrganizationId: 10,
		Name:           "Archived",
		Description:    "Archived project",
	}).Error)

	emailer := &testEmailer{}
	postgres := &testPostgresConnector{db: db}
	return &webProjectGRPCApi{
		webProjectApi: webProjectApi{
			cfg: &web_config.WebAppConfig{
				AppConfig: app_config.AppConfig{
					Ui: app_config.ServiceHostConfig{Host: "http://ui.test"},
				},
			},
			logger:         logger,
			postgres:       postgres,
			projectService: internal_project_service.NewProjectService(logger, postgres),
			userService:    internal_user_service.NewUserService(logger, postgres),
			emailerClient:  emailer,
		},
	}, db, emailer
}

func logger2Discard() logger.Interface {
	return logger.Discard.LogMode(logger.Silent)
}

func ownerContext(role string) context.Context {
	return context.WithValue(context.Background(), types.CTX_, &types.PlainAuthPrinciple{
		User: types.UserInfo{
			Id:    1,
			Name:  "Owner",
			Email: "owner@example.com",
		},
		OrganizationRole: &types.OrganizaitonRole{
			Id:               1,
			OrganizationId:   10,
			Role:             role,
			OrganizationName: "Acme",
		},
	})
}

func TestAddUsersToProjectRejectsAuthAndValidationFailures(t *testing.T) {
	api, db, _ := newProjectAPITest(t)

	_, err := api.AddUsersToProject(context.Background(), &protos.AddUsersToProjectRequest{
		Email:      "new@example.com",
		Role:       type_enums.PROJECT_ROLE_READER.String(),
		ProjectIds: []uint64{100},
	})
	require.Error(t, err)

	tests := []struct {
		name string
		ctx  context.Context
		req  *protos.AddUsersToProjectRequest
	}{
		{
			name: "non admin",
			ctx:  ownerContext(type_enums.ORGANIZATION_ROLE_MEMBER.String()),
			req: &protos.AddUsersToProjectRequest{
				Email:      "new@example.com",
				Role:       type_enums.PROJECT_ROLE_READER.String(),
				ProjectIds: []uint64{100},
			},
		},
		{
			name: "invalid raw email",
			ctx:  ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()),
			req: &protos.AddUsersToProjectRequest{
				Email:      " new@example.com",
				Role:       type_enums.PROJECT_ROLE_READER.String(),
				ProjectIds: []uint64{100},
			},
		},
		{
			name: "invalid role",
			ctx:  ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()),
			req: &protos.AddUsersToProjectRequest{
				Email:      "new@example.com",
				Role:       "Reader",
				ProjectIds: []uint64{100},
			},
		},
		{
			name: "empty project list",
			ctx:  ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()),
			req: &protos.AddUsersToProjectRequest{
				Email:      "new@example.com",
				Role:       type_enums.PROJECT_ROLE_READER.String(),
				ProjectIds: nil,
			},
		},
		{
			name: "empty project id",
			ctx:  ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()),
			req: &protos.AddUsersToProjectRequest{
				Email:      "new@example.com",
				Role:       type_enums.PROJECT_ROLE_READER.String(),
				ProjectIds: []uint64{0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := api.AddUsersToProject(tt.ctx, tt.req)
			require.Error(t, err)
			var count int64
			require.NoError(t, db.Model(&internal_entity.UserProjectRole{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestAddUsersToProjectRejectsInvalidProjectsBeforeWrites(t *testing.T) {
	api, db, _ := newProjectAPITest(t)

	for _, ids := range [][]uint64{{200}, {300}, {999}, {100, 200}} {
		_, err := api.AddUsersToProject(ownerContext(type_enums.ORGANIZATION_ROLE_ADMIN.String()), &protos.AddUsersToProjectRequest{
			Email:      "new@example.com",
			Role:       type_enums.PROJECT_ROLE_WRITER.String(),
			ProjectIds: ids,
		})
		require.Error(t, err)
		var count int64
		require.NoError(t, db.Model(&internal_entity.UserProjectRole{}).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestAddUsersToProjectInvitesNewUserAsMemberWithProjectRole(t *testing.T) {
	api, db, emailer := newProjectAPITest(t)

	res, err := api.AddUsersToProject(ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()), &protos.AddUsersToProjectRequest{
		Email:      "new@example.com",
		Role:       type_enums.PROJECT_ROLE_WRITER.String(),
		ProjectIds: []uint64{100, 101},
	})
	require.NoError(t, err)
	require.True(t, res.GetSuccess())
	require.Len(t, res.GetData(), 2)
	require.Equal(t, 1, emailer.calls)
	require.Equal(t, "new@example.com", emailer.to.Email)
	require.Equal(t, "Alpha,Beta", emailer.args["project_name"])

	var user internal_entity.UserAuth
	require.NoError(t, db.First(&user, "email = ?", "new@example.com").Error)
	var orgRole internal_entity.UserOrganizationRole
	require.NoError(t, db.First(&orgRole, "user_auth_id = ?", user.Id).Error)
	require.Equal(t, type_enums.ORGANIZATION_ROLE_MEMBER.String(), orgRole.Role)
	require.Equal(t, type_enums.RECORD_INVITED, orgRole.Status)
	var projectRoles []internal_entity.UserProjectRole
	require.NoError(t, db.Find(&projectRoles, "user_auth_id = ?", user.Id).Error)
	require.Len(t, projectRoles, 2)
	for _, projectRole := range projectRoles {
		require.Equal(t, type_enums.PROJECT_ROLE_WRITER.String(), projectRole.Role)
		require.Equal(t, type_enums.RECORD_INVITED, projectRole.Status)
	}
}

func TestAddUsersToProjectUpdatesExistingSameOrgProjectRole(t *testing.T) {
	api, db, _ := newProjectAPITest(t)
	require.NoError(t, db.Create(&internal_entity.UserAuth{
		Audited: gorm_models.Audited{Id: 50},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ACTIVE,
			CreatedBy: 1,
		},
		Name:     "Existing",
		Email:    "existing@example.com",
		Password: "hash",
		Source:   "direct",
	}).Error)
	require.NoError(t, db.Create(&internal_entity.UserOrganizationRole{
		Audited: gorm_models.Audited{Id: 51},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ACTIVE,
			CreatedBy: 1,
		},
		UserAuthId:     50,
		OrganizationId: 10,
		Role:           type_enums.ORGANIZATION_ROLE_MEMBER.String(),
	}).Error)
	require.NoError(t, db.Create(&internal_entity.UserProjectRole{
		Audited: gorm_models.Audited{Id: 52},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_ACTIVE,
			CreatedBy: 1,
		},
		UserAuthId: 50,
		ProjectId:  100,
		Role:       type_enums.PROJECT_ROLE_READER.String(),
	}).Error)

	res, err := api.AddUsersToProject(ownerContext(type_enums.ORGANIZATION_ROLE_ADMIN.String()), &protos.AddUsersToProjectRequest{
		Email:      "existing@example.com",
		Role:       type_enums.PROJECT_ROLE_ADMIN.String(),
		ProjectIds: []uint64{100},
	})
	require.NoError(t, err)
	require.True(t, res.GetSuccess())

	var projectRoles []internal_entity.UserProjectRole
	require.NoError(t, db.Find(&projectRoles, "user_auth_id = ? AND project_id = ?", 50, 100).Error)
	require.Len(t, projectRoles, 1)
	require.Equal(t, type_enums.PROJECT_ROLE_ADMIN.String(), projectRoles[0].Role)
	require.Equal(t, type_enums.RECORD_ACTIVE, projectRoles[0].Status)
}

func TestAddUsersToProjectReusesExistingInvitedOrganizationRole(t *testing.T) {
	api, db, _ := newProjectAPITest(t)
	require.NoError(t, db.Create(&internal_entity.UserAuth{
		Audited: gorm_models.Audited{Id: 60},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_INVITED,
			CreatedBy: 1,
		},
		Name:     "Invited",
		Email:    "invited@example.com",
		Password: "hash",
		Source:   "invited-by-other",
	}).Error)
	require.NoError(t, db.Create(&internal_entity.UserOrganizationRole{
		Audited: gorm_models.Audited{Id: 61},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_INVITED,
			CreatedBy: 1,
		},
		UserAuthId:     60,
		OrganizationId: 10,
		Role:           type_enums.ORGANIZATION_ROLE_MEMBER.String(),
	}).Error)

	res, err := api.AddUsersToProject(ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()), &protos.AddUsersToProjectRequest{
		Email:      "invited@example.com",
		Role:       type_enums.PROJECT_ROLE_READER.String(),
		ProjectIds: []uint64{100},
	})
	require.NoError(t, err)
	require.True(t, res.GetSuccess())

	var orgRoleCount int64
	require.NoError(t, db.Model(&internal_entity.UserOrganizationRole{}).Where("user_auth_id = ?", 60).Count(&orgRoleCount).Error)
	require.EqualValues(t, 1, orgRoleCount)
	var projectRole internal_entity.UserProjectRole
	require.NoError(t, db.First(&projectRole, "user_auth_id = ? AND project_id = ?", 60, 100).Error)
	require.Equal(t, type_enums.PROJECT_ROLE_READER.String(), projectRole.Role)
	require.Equal(t, type_enums.RECORD_INVITED, projectRole.Status)
}

func TestAddUsersToProjectRejectsExistingInvitedUserInAnotherOrganization(t *testing.T) {
	api, db, emailer := newProjectAPITest(t)
	require.NoError(t, db.Create(&internal_entity.UserAuth{
		Audited: gorm_models.Audited{Id: 70},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_INVITED,
			CreatedBy: 1,
		},
		Name:     "Invited",
		Email:    "cross-invited@example.com",
		Password: "hash",
		Source:   "invited-by-other",
	}).Error)
	require.NoError(t, db.Create(&internal_entity.UserOrganizationRole{
		Audited: gorm_models.Audited{Id: 71},
		Mutable: gorm_models.Mutable{
			Status:    type_enums.RECORD_INVITED,
			CreatedBy: 1,
		},
		UserAuthId:     70,
		OrganizationId: 20,
		Role:           type_enums.ORGANIZATION_ROLE_MEMBER.String(),
	}).Error)

	_, err := api.AddUsersToProject(ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()), &protos.AddUsersToProjectRequest{
		Email:      "cross-invited@example.com",
		Role:       type_enums.PROJECT_ROLE_READER.String(),
		ProjectIds: []uint64{100},
	})
	require.Error(t, err)
	require.Zero(t, emailer.calls)

	var projectRoleCount int64
	require.NoError(t, db.Model(&internal_entity.UserProjectRole{}).Where("user_auth_id = ?", 70).Count(&projectRoleCount).Error)
	require.Zero(t, projectRoleCount)
}

func TestAddUsersToProjectProjectRoleFailureReturnsError(t *testing.T) {
	api, db, _ := newProjectAPITest(t)
	require.NoError(t, db.Exec(`DROP TABLE user_project_roles`).Error)

	_, err := api.AddUsersToProject(ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()), &protos.AddUsersToProjectRequest{
		Email:      "new@example.com",
		Role:       type_enums.PROJECT_ROLE_READER.String(),
		ProjectIds: []uint64{100},
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&internal_entity.UserAuth{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestAddUsersToProjectEmailFailureDoesNotRollback(t *testing.T) {
	api, db, emailer := newProjectAPITest(t)
	emailer.err = errors.New("email failed")

	res, err := api.AddUsersToProject(ownerContext(type_enums.ORGANIZATION_ROLE_OWNER.String()), &protos.AddUsersToProjectRequest{
		Email:      "new@example.com",
		Role:       type_enums.PROJECT_ROLE_READER.String(),
		ProjectIds: []uint64{100},
	})
	require.NoError(t, err)
	require.True(t, res.GetSuccess())
	require.Equal(t, 1, emailer.calls)

	var count int64
	require.NoError(t, db.Model(&internal_entity.UserProjectRole{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
