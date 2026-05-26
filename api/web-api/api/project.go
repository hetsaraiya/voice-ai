package web_api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rapidaai/api/web-api/config"
	"github.com/rapidaai/pkg/ciphers"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/pkg/connectors"
	"github.com/rapidaai/pkg/types"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
	"github.com/rapidaai/protos"

	internal_entity "github.com/rapidaai/api/web-api/internal/entity"
	internal_service "github.com/rapidaai/api/web-api/internal/service"
	internal_organization_service "github.com/rapidaai/api/web-api/internal/service/organization"
	internal_project_service "github.com/rapidaai/api/web-api/internal/service/project"
	internal_user_service "github.com/rapidaai/api/web-api/internal/service/user"
	external_clients "github.com/rapidaai/pkg/clients/external"
	external_emailer "github.com/rapidaai/pkg/clients/external/emailer"
	external_emailer_template "github.com/rapidaai/pkg/clients/external/emailer/template"
	type_enums "github.com/rapidaai/pkg/types/enums"
)

type webProjectApi struct {
	cfg                 *config.WebAppConfig
	logger              commons.Logger
	redis               connectors.RedisConnector
	postgres            connectors.PostgresConnector
	projectService      internal_service.ProjectService
	emailerClient       external_clients.Emailer
	userService         internal_service.UserService
	organizationService internal_service.OrganizationService
}

type webProjectRPCApi struct {
	webProjectApi
}

type webProjectGRPCApi struct {
	webProjectApi
}

func NewProjectRPC(config *config.WebAppConfig, logger commons.Logger, postgres connectors.PostgresConnector, redis connectors.RedisConnector) *webProjectRPCApi {
	return &webProjectRPCApi{
		webProjectApi{
			cfg:            config,
			logger:         logger,
			postgres:       postgres,
			redis:          redis,
			projectService: internal_project_service.NewProjectService(logger, postgres),
			emailerClient:  external_emailer.NewEmailer(config.EmailerConfig, logger),
		},
	}
}

func NewProjectGRPC(config *config.WebAppConfig, logger commons.Logger, postgres connectors.PostgresConnector, redis connectors.RedisConnector) protos.ProjectServiceServer {
	return &webProjectGRPCApi{
		webProjectApi{
			cfg:                 config,
			logger:              logger,
			postgres:            postgres,
			redis:               redis,
			projectService:      internal_project_service.NewProjectService(logger, postgres),
			userService:         internal_user_service.NewUserService(logger, postgres),
			emailerClient:       external_emailer.NewEmailer(config.EmailerConfig, logger),
			organizationService: internal_organization_service.NewOrganizationService(logger, postgres),
		},
	}
}

func (wProjectApi *webProjectGRPCApi) CreateProject(ctx context.Context, irRequest *protos.CreateProjectRequest) (*protos.CreateProjectResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(ctx)
	if !isAuthenticated {
		wProjectApi.logger.Errorf("CreateProject from grpc with unauthenticated request")
		return nil, errors.New("unauthenticated request")
	}
	currentOrgRole := iAuth.GetOrganizationRole()
	if currentOrgRole == nil {
		wProjectApi.logger.Errorf("current org is null, you can't create project without an organization.")
		return utils.Error[protos.CreateProjectResponse](
			errors.New("you cannot create a project when you are not part of any organization"),
			"Please create organization before creating a project.")
	}

	prj, err := wProjectApi.projectService.Create(ctx, iAuth, iAuth.GetOrganizationRole().OrganizationId, irRequest.GetProjectName(), irRequest.GetProjectDescription())
	if err != nil {
		wProjectApi.logger.Errorf("projectService.Create from grpc with err %v", err)
		return utils.Error[protos.CreateProjectResponse](
			err,
			"Unable to create project for your organization, please try again in sometime")
	}

	_, err = wProjectApi.userService.CreateProjectRole(ctx, iAuth, iAuth.GetUserInfo().Id, type_enums.PROJECT_ROLE_ADMIN.String(), prj.Id, type_enums.RECORD_ACTIVE)
	if err != nil {
		wProjectApi.logger.Errorf("userService.CreateProjectRole from grpc with err %v", err)
		return utils.Error[protos.CreateProjectResponse](
			err, "Unable to create project role for you, please try again in sometime")
	}
	ot := &protos.Project{}
	err = utils.Cast(prj, ot)
	if err != nil {
		wProjectApi.logger.Errorf("unable to cast project to proto object %v", err)
	}
	return utils.Success[protos.CreateProjectResponse, *protos.Project](ot)
}

/*
update project request
*/
func (wProjectApi *webProjectGRPCApi) UpdateProject(ctx context.Context, irRequest *protos.UpdateProjectRequest) (*protos.UpdateProjectResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(ctx)
	if !isAuthenticated {
		wProjectApi.logger.Errorf("UpdateProject from grpc with unauthenticated request")
		return nil, errors.New("unauthenticated request")
	}

	currentOrgRole := iAuth.GetOrganizationRole()
	if currentOrgRole == nil {
		wProjectApi.logger.Errorf("current org is not null, you can't create multiple organization at same time.")
		return utils.Error[protos.UpdateProjectResponse](
			errors.New("you cannot update a project when you are not part of any organization"),
			"Please create organization before updating a project.")
	}

	prj, err := wProjectApi.projectService.Update(ctx, iAuth, irRequest.GetProjectId(), irRequest.ProjectName, irRequest.ProjectDescription)
	if err != nil {
		wProjectApi.logger.Errorf("projectService.Update from grpc with err %v", err)
		return utils.Error[protos.UpdateProjectResponse](err,
			"Unable to update the project, please try again in sometime.")
	}

	ot := &protos.Project{}
	err = utils.Cast(prj, ot)
	if err != nil {
		wProjectApi.logger.Errorf("unable to cast project to proto object %v", err)
	}

	return utils.Success[protos.UpdateProjectResponse, *protos.Project](ot)
}
func (wProjectApi *webProjectGRPCApi) GetAllProject(ctx context.Context, irRequest *protos.GetAllProjectRequest) (*protos.GetAllProjectResponse, error) {
	iAuth, isAuthenticated := types.GetAuthPrincipleGPRC(ctx)
	if !isAuthenticated {
		wProjectApi.logger.Errorf("GetAllProject from grpc with unauthenticated request")
		return nil, errors.New("unauthenticated request")
	}

	currentOrgRole := iAuth.GetOrganizationRole()
	if currentOrgRole == nil {
		wProjectApi.logger.Errorf("current org is not null, you can't create multiple organization at same time.")
		return utils.Error[protos.GetAllProjectResponse](
			errors.New("you are not part of any active organization"),
			"Please create organization and try again.",
		)
	}

	cnt, prjs, err := wProjectApi.projectService.GetAll(ctx, iAuth,
		currentOrgRole.OrganizationId, irRequest.GetCriterias(), irRequest.GetPaginate())
	if err != nil {
		wProjectApi.logger.Errorf("projectService.GetAll from grpc with err %v", err)
		return utils.Error[protos.GetAllProjectResponse](
			err,
			"Unable to get the projects, please try again in sometime.",
		)
	}

	out := []*protos.Project{}
	err = utils.Cast(prjs, &out)
	if err != nil {
		wProjectApi.logger.Errorf("unable to cast project to proto object %v", err)
	}

	for _, prj := range out {
		_m, err := wProjectApi.userService.GetAllActiveProjectMember(ctx, prj.Id)
		if err != nil {
			wProjectApi.logger.Errorf("no member in the project %v with err %v", prj.Id, err)
			continue
		}
		for _, upr := range _m {
			prj.Members = append(prj.Members, &protos.User{
				Role:  upr.Role,
				Id:    upr.UserAuthId,
				Name:  upr.Member.Name,
				Email: upr.Member.Email,
			})
		}
	}
	return utils.PaginatedSuccess[protos.GetAllProjectResponse, []*protos.Project](uint32(cnt), irRequest.GetPaginate().GetPage(), out)
}

func (wProjectApi *webProjectGRPCApi) GetProject(ctx context.Context, irRequest *protos.GetProjectRequest) (*protos.GetProjectResponse, error) {
	iAuth, isAuthenticated := types.GetSimplePrincipleGRPC(ctx)
	if !isAuthenticated {
		wProjectApi.logger.Errorf("GetProject from grpc with unauthenticated request")
		return nil, errors.New("unauthenticated request")
	}

	if irRequest.GetProjectId() == 0 {
		return utils.Error[protos.GetProjectResponse](
			errors.New("projectid is not getting passed"),
			"Please select the project to see the details.",
		)
	}

	prj, err := wProjectApi.projectService.Get(ctx, iAuth, irRequest.GetProjectId())
	if err != nil {
		wProjectApi.logger.Errorf("projectService.Get from grpc with err %v", err)
		return utils.Error[protos.GetProjectResponse](
			err,
			"Please select the project to see the details.",
		)
	}

	ot := &protos.Project{}
	utils.Cast(prj, ot)
	var projectMemebers []*internal_entity.UserProjectRole
	projectMemebers, err = wProjectApi.userService.GetAllActiveProjectMember(ctx, prj.Id)
	if err != nil {
		wProjectApi.logger.Errorf("userService.GetAllProjectMember from grpc with err %v", err)
		return nil, err
	}

	projectMembers := make([]*protos.User, len(projectMemebers))
	for idx, upr := range projectMemebers {
		projectMembers[idx] = &protos.User{
			Role:  upr.Role,
			Id:    upr.UserAuthId,
			Name:  upr.Member.Name,
			Email: upr.Member.Email,
		}
	}

	ot.Members = projectMembers
	return utils.Success[protos.GetProjectResponse, *protos.Project](ot)
}

func (wProjectApi *webProjectGRPCApi) AddUsersToProject(ctx context.Context, irRequest *protos.AddUsersToProjectRequest) (*protos.AddUsersToProjectResponse, error) {
	auth, isAuthenticated := types.GetAuthPrincipleGPRC(ctx)
	if !isAuthenticated {
		return nil, errors.New("unauthenticated request")
	}
	currentOrgRole := auth.GetOrganizationRole()
	if currentOrgRole == nil {
		return utils.Error[protos.AddUsersToProjectResponse](
			errors.New("you are not part of any active organization"),
			"Please create organization and try again.",
		)
	}
	if !validator.OneOf(currentOrgRole.Role, type_enums.ORGANIZATION_ROLE_OWNER.String(), type_enums.ORGANIZATION_ROLE_ADMIN.String()) {
		return utils.Error[protos.AddUsersToProjectResponse](
			errors.New("user is not authorized to invite users to projects"),
			"You do not have permission to invite users to projects.",
		)
	}

	if !validator.Email(irRequest.GetEmail()) {
		return utils.Error[protos.AddUsersToProjectResponse](
			errors.New("invalid email address"),
			"The provided email is not valid, please check the email and retry.",
		)
	}
	if !validator.NotEmpty(irRequest.GetProjectIds()) {
		return utils.Error[protos.AddUsersToProjectResponse](
			errors.New("project ids are required"),
			"Please select at least one project and retry.",
		)
	}
	if !validator.AllNonZero(irRequest.GetProjectIds()...) {
		return utils.Error[protos.AddUsersToProjectResponse](
			errors.New("project ids must be non-empty"),
			"Please select valid projects and retry.",
		)
	}
	if !validator.OneOf(irRequest.GetRole(), type_enums.PROJECT_ROLE_SUPER_ADMIN.String(), type_enums.PROJECT_ROLE_ADMIN.String(), type_enums.PROJECT_ROLE_WRITER.String(), type_enums.PROJECT_ROLE_READER.String()) {
		return utils.Error[protos.AddUsersToProjectResponse](
			errors.New("invalid project role"),
			"Please select a valid project role and retry.",
		)
	}

	projects, err := wProjectApi.projectService.GetAllByOrganization(ctx, auth, currentOrgRole.OrganizationId, utils.Unique(irRequest.GetProjectIds()))
	if err != nil {
		wProjectApi.logger.Errorf("projectService.GetAllByOrganization from grpc with err %v", err)
		return utils.Error[protos.AddUsersToProjectResponse](
			err,
			"Please select valid projects and retry.",
		)
	}
	projectIds := make([]uint64, 0, len(projects))
	projectNames := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIds = append(projectIds, project.Id)
		projectNames = append(projectNames, project.Name)
	}

	eUser, err := wProjectApi.userService.Get(ctx, irRequest.GetEmail())
	if err != nil {
		source := "invited-by-other"
		parts := strings.Split(irRequest.GetEmail(), "@")
		// user creation for invite, we will create a random password and send email to reset password flow
		ePrinciple, err := wProjectApi.userService.Create(ctx, parts[0], irRequest.GetEmail(), ciphers.RandomHash("rpd_"), type_enums.RECORD_INVITED, &source)
		if err != nil {
			wProjectApi.logger.Errorf("unable to create user for invite err %v", err)
			return utils.Error[protos.AddUsersToProjectResponse](
				err,
				"Unable to add user to project, please try again in sometime.",
			)
		}

		// user is newly created for invite, so we will create organization role for the user
		_, err = wProjectApi.userService.CreateOrganizationRole(ctx, auth, type_enums.ORGANIZATION_ROLE_MEMBER.String(), *ePrinciple.GetUserId(), currentOrgRole.OrganizationId, type_enums.RECORD_INVITED)
		if err != nil {
			wProjectApi.logger.Errorf("unable to create organization role err %v", err)
			return utils.Error[protos.AddUsersToProjectResponse](
				err,
				"Unable to add user to project, please try again in sometime.",
			)
		}
		_, err = wProjectApi.userService.CreateProjectRoles(ctx, auth, *ePrinciple.GetUserId(), irRequest.GetRole(), projectIds, type_enums.RECORD_INVITED)
		if err != nil {
			wProjectApi.logger.Errorf("unable to create project role for invite err %v", err)
			return utils.Error[protos.AddUsersToProjectResponse](
				err,
				"Unable to add user to project, please try again in sometime.",
			)
		}
		if err = wProjectApi.emailerClient.EmailRichText(
			ctx,
			external_clients.Contact{
				Name:  "",
				Email: irRequest.GetEmail(),
			},
			fmt.Sprintf("[RapidaAI] %s has invited you to join the %s organization", auth.GetUserInfo().Name, currentOrgRole.OrganizationName),
			external_emailer_template.INVITE_MEMBER_TEMPLATE,
			map[string]string{
				"inviter_name": auth.GetUserInfo().Name,
				"project_name": strings.Join(projectNames, ","),
				"invite_url":   fmt.Sprintf("%s/auth/signup?utm_source=invite&utm_param=%d", wProjectApi.cfg.BaseUrl(), currentOrgRole.OrganizationId),
			},
		); err != nil {
			wProjectApi.logger.Errorf("error while sending invite email %v", err)
		}

	} else {
		org, err := wProjectApi.userService.GetAnyOrganizationRole(ctx, eUser.GetId())
		if err != nil {
			_, err = wProjectApi.userService.CreateOrganizationRole(ctx, auth, type_enums.ORGANIZATION_ROLE_MEMBER.String(), eUser.GetId(), currentOrgRole.OrganizationId, eUser.Status)
			if err != nil {
				wProjectApi.logger.Errorf("unable to create organization role err %v", err)
				return utils.Error[protos.AddUsersToProjectResponse](
					err,
					"Unable to add user to project, please try again in sometime.",
				)
			}
		} else if org.GetOrganizationId() != currentOrgRole.OrganizationId {
			return utils.Error[protos.AddUsersToProjectResponse](
				errors.New("user is already part of another organization"),
				"User is part of another organization, please ask the user to switch to this organization before adding to project.",
			)
		}

		_, err = wProjectApi.userService.CreateProjectRoles(ctx, auth, eUser.Id, irRequest.GetRole(), projectIds, eUser.Status)
		if err != nil {
			wProjectApi.logger.Errorf("unable to create project role for invite err %v", err)
			return utils.Error[protos.AddUsersToProjectResponse](
				err,
				"Unable to add user to project, please try again in sometime.",
			)
		}
		if err = wProjectApi.emailerClient.EmailRichText(
			ctx,
			external_clients.Contact{
				Name:  "",
				Email: irRequest.GetEmail(),
			},
			fmt.Sprintf("[RapidaAI] %s has invited you to join the %s organization", auth.GetUserInfo().Name, currentOrgRole.OrganizationName),
			external_emailer_template.INVITE_MEMBER_TEMPLATE,
			map[string]string{
				"inviter_name": auth.GetUserInfo().Name,
				"project_name": strings.Join(projectNames, ","),
				"invite_url":   fmt.Sprintf("%s/auth/signup?utm_source=invite&utm_param=%d", wProjectApi.cfg.BaseUrl(), currentOrgRole.OrganizationId),
			},
		); err != nil {
			wProjectApi.logger.Errorf("error while sending invite email %v", err)
		}

	}
	out := []*protos.Project{}
	err = utils.Cast(projects, &out)
	if err != nil {
		wProjectApi.logger.Errorf("unable to cast project credential to proto object %v", err)
	}
	return utils.Success[protos.AddUsersToProjectResponse, []*protos.Project](out)

}

/*
This api will be for future
if you are reading one of the example that you waste time writing code
*/
func (wProjectApi *webProjectGRPCApi) ArchiveProject(c context.Context, irRequest *protos.ArchiveProjectRequest) (*protos.ArchiveProjectResponse, error) {
	wProjectApi.logger.Debugf("ArchiveProjectRequest from grpc with requestPayload %v, %v", irRequest, c)
	auth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		wProjectApi.logger.Errorf("DeleteProviderCredential from grpc with unauthenticated request")
		return nil, errors.New("unauthenticated request")
	}
	if _, err := wProjectApi.projectService.Archive(c, auth, irRequest.Id); err != nil {
		wProjectApi.logger.Errorf("DeleteProviderCredential while archieving project")
		return nil, err
	}
	return utils.Success[protos.ArchiveProjectResponse, uint64](irRequest.Id)
}

func (wProjectApi *webProjectGRPCApi) CreateProjectCredential(c context.Context, irRequest *protos.CreateProjectCredentialRequest) (*protos.CreateProjectCredentialResponse, error) {
	auth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		wProjectApi.logger.Errorf("CreateProjectCredential from grpc with unauthenticated request")
		return nil, errors.New("unauthenticated request")
	}

	// name, key string, projectId, organizationId uint64
	pc, err := wProjectApi.projectService.CreateCredential(c, auth, irRequest.GetName(), irRequest.GetProjectId(), auth.GetOrganizationRole().OrganizationId)
	if err != nil {
		return utils.Error[protos.CreateProjectCredentialResponse](
			err,
			"Unable to create the project credential, please try again in sometime.",
		)
	}
	out := &protos.ProjectCredential{}
	err = utils.Cast(pc, &out)
	if err != nil {
		wProjectApi.logger.Errorf("unable to cast project credential to proto object %v", err)
	}
	return utils.Success[protos.CreateProjectCredentialResponse, *protos.ProjectCredential](out)
}

func (wProjectApi *webProjectGRPCApi) GetAllProjectCredential(c context.Context, irRequest *protos.GetAllProjectCredentialRequest) (*protos.GetAllProjectCredentialResponse, error) {
	auth, isAuthenticated := types.GetAuthPrincipleGPRC(c)
	if !isAuthenticated {
		wProjectApi.logger.Errorf("CreateProjectCredential from grpc with unauthenticated request")
		return nil, errors.New("unauthenticated request")
	}

	// name, key string, projectId, organizationId uint64
	cnt, allProjectCredential, err := wProjectApi.projectService.
		GetAllCredential(
			c, auth,
			irRequest.GetProjectId(),
			auth.GetOrganizationRole().OrganizationId,
			irRequest.GetCriterias(), irRequest.GetPaginate())
	if err != nil {
		return utils.Error[protos.GetAllProjectCredentialResponse](
			err,
			"Unable to get all the project credentials, please try again in sometime.",
		)
	}

	out := []*protos.ProjectCredential{}
	err = utils.Cast(allProjectCredential, &out)
	if err != nil {
		wProjectApi.logger.Errorf("unable to cast project credential to proto object %v", err)
	}

	return utils.PaginatedSuccess[protos.GetAllProjectCredentialResponse, []*protos.ProjectCredential](
		uint32(cnt),
		irRequest.GetPaginate().GetPage(),
		out)
}
