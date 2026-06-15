package assistant_deployment_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	internal_services "github.com/rapidaai/api/assistant-api/internal/services"
	"github.com/rapidaai/pkg/commons"
	pkg_errors "github.com/rapidaai/pkg/errors"
	"github.com/rapidaai/pkg/types"
	type_enums "github.com/rapidaai/pkg/types/enums"
	"github.com/rapidaai/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type createDebuggerDeploymentRestServiceStub struct {
	createCalled       bool
	createErr          error
	assistantId        uint64
	greeting           *string
	inputAudio         *protos.DeploymentAudioProvider
	maxSessionDuration *uint64
}

func (s *createDebuggerDeploymentRestServiceStub) CreateWhatsappDeployment(
	context.Context,
	types.SimplePrinciple,
	uint64,
	*string,
	*string,
	*uint64,
	*uint64,
	*string,
	*uint64,
	string,
	[]*protos.Metadata,
) (*internal_assistant_entity.AssistantWhatsappDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) CreatePhoneDeployment(
	context.Context,
	types.SimplePrinciple,
	uint64,
	*string,
	*string,
	*uint64,
	*uint64,
	*string,
	*uint64,
	string,
	*protos.DeploymentAudioProvider,
	*protos.DeploymentAudioProvider,
	[]*protos.Metadata,
) (*internal_assistant_entity.AssistantPhoneDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) CreateApiDeployment(
	context.Context,
	types.SimplePrinciple,
	uint64,
	*string,
	*string,
	*uint64,
	*uint64,
	*string,
	*uint64,
	*protos.DeploymentAudioProvider,
	*protos.DeploymentAudioProvider,
) (*internal_assistant_entity.AssistantApiDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) CreateDebuggerDeployment(
	_ context.Context,
	_ types.SimplePrinciple,
	assistantId uint64,
	greeting, _ *string,
	idealTimeout *uint64,
	_ *uint64,
	_ *string,
	maxSessionDuration *uint64,
	inputAudio, _ *protos.DeploymentAudioProvider,
) (*internal_assistant_entity.AssistantDebuggerDeployment, error) {
	s.createCalled = true
	s.assistantId = assistantId
	s.greeting = greeting
	s.inputAudio = inputAudio
	s.maxSessionDuration = maxSessionDuration
	if s.createErr != nil {
		return nil, s.createErr
	}

	inputAudioEntity := deploymentAudioProviderEntityFromProto(inputAudio)
	return &internal_assistant_entity.AssistantDebuggerDeployment{
		AssistantDeploymentBehavior: internal_assistant_entity.AssistantDeploymentBehavior{
			AssistantDeployment: internal_assistant_entity.AssistantDeployment{
				AssistantId: assistantId,
			},
			Greeting:           greeting,
			IdleTimeout:        idealTimeout,
			MaxSessionDuration: maxSessionDuration,
		},
		InputAudio: inputAudioEntity,
	}, nil
}

func (s *createDebuggerDeploymentRestServiceStub) CreateWebPluginDeployment(
	context.Context,
	types.SimplePrinciple,
	uint64,
	*string,
	*string,
	*uint64,
	*uint64,
	*string,
	*uint64,
	[]string,
	*protos.DeploymentAudioProvider,
	*protos.DeploymentAudioProvider,
) (*internal_assistant_entity.AssistantWebPluginDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAssistantApiDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantApiDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAssistantDebuggerDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantDebuggerDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAssistantPhoneDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantPhoneDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAssistantWebpluginDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantWebPluginDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAssistantWhatsappDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantWhatsappDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAllAssistantApiDeployment(context.Context, types.SimplePrinciple, uint64, []*protos.Criteria, *protos.Paginate) (int64, []*internal_assistant_entity.AssistantApiDeployment, error) {
	return 0, nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAllAssistantDebuggerDeployment(context.Context, types.SimplePrinciple, uint64, []*protos.Criteria, *protos.Paginate) (int64, []*internal_assistant_entity.AssistantDebuggerDeployment, error) {
	return 0, nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAllAssistantPhoneDeployment(context.Context, types.SimplePrinciple, uint64, []*protos.Criteria, *protos.Paginate) (int64, []*internal_assistant_entity.AssistantPhoneDeployment, error) {
	return 0, nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAllAssistantWebpluginDeployment(context.Context, types.SimplePrinciple, uint64, []*protos.Criteria, *protos.Paginate) (int64, []*internal_assistant_entity.AssistantWebPluginDeployment, error) {
	return 0, nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) GetAllAssistantWhatsappDeployment(context.Context, types.SimplePrinciple, uint64, []*protos.Criteria, *protos.Paginate) (int64, []*internal_assistant_entity.AssistantWhatsappDeployment, error) {
	return 0, nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) DisableAssistantApiDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantApiDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) DisableAssistantDebuggerDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantDebuggerDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) DisableAssistantPhoneDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantPhoneDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) DisableAssistantWebpluginDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantWebPluginDeployment, error) {
	return nil, errors.New("not implemented")
}

func (s *createDebuggerDeploymentRestServiceStub) DisableAssistantWhatsappDeployment(context.Context, types.SimplePrinciple, uint64) (*internal_assistant_entity.AssistantWhatsappDeployment, error) {
	return nil, errors.New("not implemented")
}

func TestCreateAssistantDebuggerDeploymentRest_HappyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &createDebuggerDeploymentRestServiceStub{}
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, service)
	requestBody := []byte(`{
		"assistantId": "123",
		"greeting": "Hello",
		"idealTimeout": 30,
		"maxSessionDuration": 600,
		"inputAudio": {
			"audioProvider": "twilio",
			"audioType": "input",
			"audioOptions": [{"key": "codec", "value": "mulaw"}]
		}
	}`)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader(requestBody),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set(string(types.CTX_), createDebuggerDeploymentRestAuth())

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, service.createCalled)
	assert.Equal(t, uint64(123), service.assistantId)
	require.NotNil(t, service.greeting)
	assert.Equal(t, "Hello", *service.greeting)
	require.NotNil(t, service.maxSessionDuration)
	assert.Equal(t, uint64(600), *service.maxSessionDuration)
	require.NotNil(t, service.inputAudio)
	assert.Equal(t, "twilio", service.inputAudio.GetAudioProvider())
	require.Len(t, service.inputAudio.GetAudioOptions(), 1)
	assert.Equal(t, "codec", service.inputAudio.GetAudioOptions()[0].GetKey())

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, true, response["success"])
	data := response["data"].(map[string]interface{})
	assert.Equal(t, "123", data["assistantId"])
}

func TestCreateAssistantDebuggerDeploymentRest_Unauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, &createDebuggerDeploymentRestServiceStub{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader([]byte(`{}`)),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), pkg_errors.CreateAssistantDebuggerDeploymentUnauthenticated.Error)
}

func TestCreateAssistantDebuggerDeploymentRest_MissingAuthScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, &createDebuggerDeploymentRestServiceStub{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader([]byte(`{"assistantId":"123"}`)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set(string(types.CTX_), &types.PlainAuthPrinciple{
		User:             types.UserInfo{Id: 11},
		OrganizationRole: &types.OrganizaitonRole{OrganizationId: 22},
	})

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), pkg_errors.CreateAssistantDebuggerDeploymentMissingAuthScope.Error)
}

func TestCreateAssistantDebuggerDeploymentRest_InvalidAssistantID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, &createDebuggerDeploymentRestServiceStub{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader([]byte(`{"assistantId":"abc"}`)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set(string(types.CTX_), createDebuggerDeploymentRestAuth())

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), pkg_errors.CreateAssistantDebuggerDeploymentInvalidAssistantID.Error)
}

func TestCreateAssistantDebuggerDeploymentRest_InvalidAudioProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, &createDebuggerDeploymentRestServiceStub{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader([]byte(`{"assistantId":"123","inputAudio":{"audioProvider":""}}`)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set(string(types.CTX_), createDebuggerDeploymentRestAuth())

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), pkg_errors.CreateAssistantDebuggerDeploymentInvalidAudioProvider.Error)
}

func TestCreateAssistantDebuggerDeploymentRest_InvalidIdealTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, &createDebuggerDeploymentRestServiceStub{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader([]byte(`{"assistantId":"123","idealTimeout":14}`)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set(string(types.CTX_), createDebuggerDeploymentRestAuth())

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), pkg_errors.CreateAssistantDebuggerDeploymentInvalidIdealTimeout.Error)
}

func TestCreateAssistantDebuggerDeploymentRest_InvalidIdealTimeoutBackoff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, &createDebuggerDeploymentRestServiceStub{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader([]byte(`{"assistantId":"123","idealTimeoutBackoff":6}`)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set(string(types.CTX_), createDebuggerDeploymentRestAuth())

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), pkg_errors.CreateAssistantDebuggerDeploymentInvalidTimeoutBackoff.Error)
}

func TestCreateAssistantDebuggerDeploymentRest_InvalidMaxSessionDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, &createDebuggerDeploymentRestServiceStub{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader([]byte(`{"assistantId":"123","maxSessionDuration":179}`)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set(string(types.CTX_), createDebuggerDeploymentRestAuth())

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), pkg_errors.CreateAssistantDebuggerDeploymentInvalidSessionDuration.Error)
}

func TestCreateAssistantDebuggerDeploymentRest_CreateDeploymentErrorDoesNotExposeInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &createDebuggerDeploymentRestServiceStub{
		createErr: errors.New("database password leaked"),
	}
	deploymentApi := newCreateDebuggerDeploymentRestApi(t, service)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/assistant-deployment/create-debugger-deployment",
		bytes.NewReader([]byte(`{"assistantId":"123"}`)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set(string(types.CTX_), createDebuggerDeploymentRestAuth())

	deploymentApi.CreateAssistantDebuggerDeploymentRest(context)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Contains(t, recorder.Body.String(), pkg_errors.CreateAssistantDebuggerDeploymentCreateDeployment.Error)
	assert.NotContains(t, recorder.Body.String(), "database password leaked")
}

func TestCreateAssistantDebuggerDeploymentGRPC_InvalidIdealTimeout(t *testing.T) {
	service := &createDebuggerDeploymentRestServiceStub{}
	deploymentApi := newCreateDebuggerDeploymentGRPCApi(t, service)
	request := createDebuggerDeploymentGRPCRequest()
	request.GetDebugger().IdealTimeout = 14

	response, err := deploymentApi.CreateAssistantDebuggerDeployment(createDebuggerDeploymentGRPCContext(), request)

	require.Error(t, err)
	require.NotNil(t, response)
	assert.False(t, service.createCalled)
	assert.Equal(t, pkg_errors.CreateAssistantDebuggerDeploymentInvalidIdealTimeout.HTTPStatusCodeInt32(), response.Code)
	require.NotNil(t, response.Error)
	assert.Equal(t, uint64(pkg_errors.CreateAssistantDebuggerDeploymentInvalidIdealTimeout.Code), response.Error.ErrorCode)
}

func TestCreateAssistantDebuggerDeploymentGRPC_InvalidIdealTimeoutBackoff(t *testing.T) {
	service := &createDebuggerDeploymentRestServiceStub{}
	deploymentApi := newCreateDebuggerDeploymentGRPCApi(t, service)
	request := createDebuggerDeploymentGRPCRequest()
	request.GetDebugger().IdealTimeoutBackoff = 6

	response, err := deploymentApi.CreateAssistantDebuggerDeployment(createDebuggerDeploymentGRPCContext(), request)

	require.Error(t, err)
	require.NotNil(t, response)
	assert.False(t, service.createCalled)
	assert.Equal(t, pkg_errors.CreateAssistantDebuggerDeploymentInvalidTimeoutBackoff.HTTPStatusCodeInt32(), response.Code)
	require.NotNil(t, response.Error)
	assert.Equal(t, uint64(pkg_errors.CreateAssistantDebuggerDeploymentInvalidTimeoutBackoff.Code), response.Error.ErrorCode)
}

func TestCreateAssistantDebuggerDeploymentGRPC_InvalidMaxSessionDuration(t *testing.T) {
	service := &createDebuggerDeploymentRestServiceStub{}
	deploymentApi := newCreateDebuggerDeploymentGRPCApi(t, service)
	request := createDebuggerDeploymentGRPCRequest()
	request.GetDebugger().MaxSessionDuration = 179

	response, err := deploymentApi.CreateAssistantDebuggerDeployment(createDebuggerDeploymentGRPCContext(), request)

	require.Error(t, err)
	require.NotNil(t, response)
	assert.False(t, service.createCalled)
	assert.Equal(t, pkg_errors.CreateAssistantDebuggerDeploymentInvalidSessionDuration.HTTPStatusCodeInt32(), response.Code)
	require.NotNil(t, response.Error)
	assert.Equal(t, uint64(pkg_errors.CreateAssistantDebuggerDeploymentInvalidSessionDuration.Code), response.Error.ErrorCode)
}

func newCreateDebuggerDeploymentRestApi(
	t *testing.T,
	service internal_services.AssistantDeploymentService,
) *AssistantDeploymentApi {
	t.Helper()
	logger, err := commons.NewApplicationLogger()
	require.NoError(t, err)
	return &AssistantDeploymentApi{
		logger:            logger,
		deploymentService: service,
	}
}

func newCreateDebuggerDeploymentGRPCApi(
	t *testing.T,
	service internal_services.AssistantDeploymentService,
) *assistantDeploymentGrpcApi {
	t.Helper()
	logger, err := commons.NewApplicationLogger()
	require.NoError(t, err)
	return &assistantDeploymentGrpcApi{
		AssistantDeploymentApi: AssistantDeploymentApi{
			logger:            logger,
			deploymentService: service,
		},
	}
}

func createDebuggerDeploymentRestAuth() *types.PlainAuthPrinciple {
	return &types.PlainAuthPrinciple{
		User: types.UserInfo{Id: 11},
		OrganizationRole: &types.OrganizaitonRole{
			OrganizationId: 22,
		},
		CurrentProjectRole: &types.ProjectRole{
			ProjectId: 33,
		},
	}
}

func createDebuggerDeploymentGRPCContext() context.Context {
	return context.WithValue(context.Background(), types.CTX_, createDebuggerDeploymentRestAuth())
}

func createDebuggerDeploymentGRPCRequest() *protos.CreateAssistantDeploymentRequest {
	return &protos.CreateAssistantDeploymentRequest{
		Deployment: &protos.CreateAssistantDeploymentRequest_Debugger{
			Debugger: &protos.AssistantDebuggerDeployment{
				AssistantId:         123,
				IdealTimeout:        30,
				IdealTimeoutBackoff: 1,
				MaxSessionDuration:  600,
			},
		},
	}
}

func deploymentAudioProviderEntityFromProto(
	audio *protos.DeploymentAudioProvider,
) *internal_assistant_entity.AssistantDeploymentAudio {
	if audio == nil {
		return nil
	}

	entity := &internal_assistant_entity.AssistantDeploymentAudio{
		AudioProvider: audio.GetAudioProvider(),
		AudioType:     audio.GetAudioType(),
	}
	entity.Status = type_enums.RECORD_ACTIVE
	for _, option := range audio.GetAudioOptions() {
		optionEntity := &internal_assistant_entity.AssistantDeploymentAudioOption{}
		optionEntity.Key = option.GetKey()
		optionEntity.Value = option.GetValue()
		entity.AudioOptions = append(entity.AudioOptions, optionEntity)
	}
	return entity
}
