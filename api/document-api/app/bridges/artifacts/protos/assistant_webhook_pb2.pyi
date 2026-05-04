import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
import app.bridges.artifacts.protos.common_pb2 as _common_pb2
from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class AssistantWebhook(_message.Message):
    __slots__ = ("id", "assistantEvents", "description", "options", "executionPriority", "assistantId", "status", "createdBy", "createdUser", "updatedBy", "updatedUser", "createdDate", "updatedDate")
    ID_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTEVENTS_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    OPTIONS_FIELD_NUMBER: _ClassVar[int]
    EXECUTIONPRIORITY_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    CREATEDBY_FIELD_NUMBER: _ClassVar[int]
    CREATEDUSER_FIELD_NUMBER: _ClassVar[int]
    UPDATEDBY_FIELD_NUMBER: _ClassVar[int]
    UPDATEDUSER_FIELD_NUMBER: _ClassVar[int]
    CREATEDDATE_FIELD_NUMBER: _ClassVar[int]
    UPDATEDDATE_FIELD_NUMBER: _ClassVar[int]
    id: int
    assistantEvents: _containers.RepeatedScalarFieldContainer[str]
    description: str
    options: _containers.RepeatedCompositeFieldContainer[_common_pb2.Metadata]
    executionPriority: int
    assistantId: int
    status: str
    createdBy: int
    createdUser: _common_pb2.User
    updatedBy: int
    updatedUser: _common_pb2.User
    createdDate: _timestamp_pb2.Timestamp
    updatedDate: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[int] = ..., assistantEvents: _Optional[_Iterable[str]] = ..., description: _Optional[str] = ..., options: _Optional[_Iterable[_Union[_common_pb2.Metadata, _Mapping]]] = ..., executionPriority: _Optional[int] = ..., assistantId: _Optional[int] = ..., status: _Optional[str] = ..., createdBy: _Optional[int] = ..., createdUser: _Optional[_Union[_common_pb2.User, _Mapping]] = ..., updatedBy: _Optional[int] = ..., updatedUser: _Optional[_Union[_common_pb2.User, _Mapping]] = ..., createdDate: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updatedDate: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class AssistantWebhookLog(_message.Message):
    __slots__ = ("id", "webhookId", "request", "response", "status", "createdDate", "updatedDate", "assistantId", "projectId", "organizationId", "assistantConversationId", "assetPrefix", "event", "responseStatus", "timeTaken", "retryCount", "httpMethod", "httpUrl")
    ID_FIELD_NUMBER: _ClassVar[int]
    WEBHOOKID_FIELD_NUMBER: _ClassVar[int]
    REQUEST_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    CREATEDDATE_FIELD_NUMBER: _ClassVar[int]
    UPDATEDDATE_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTID_FIELD_NUMBER: _ClassVar[int]
    PROJECTID_FIELD_NUMBER: _ClassVar[int]
    ORGANIZATIONID_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTCONVERSATIONID_FIELD_NUMBER: _ClassVar[int]
    ASSETPREFIX_FIELD_NUMBER: _ClassVar[int]
    EVENT_FIELD_NUMBER: _ClassVar[int]
    RESPONSESTATUS_FIELD_NUMBER: _ClassVar[int]
    TIMETAKEN_FIELD_NUMBER: _ClassVar[int]
    RETRYCOUNT_FIELD_NUMBER: _ClassVar[int]
    HTTPMETHOD_FIELD_NUMBER: _ClassVar[int]
    HTTPURL_FIELD_NUMBER: _ClassVar[int]
    id: int
    webhookId: int
    request: _struct_pb2.Struct
    response: _struct_pb2.Struct
    status: str
    createdDate: _timestamp_pb2.Timestamp
    updatedDate: _timestamp_pb2.Timestamp
    assistantId: int
    projectId: int
    organizationId: int
    assistantConversationId: int
    assetPrefix: str
    event: str
    responseStatus: int
    timeTaken: int
    retryCount: int
    httpMethod: str
    httpUrl: str
    def __init__(self, id: _Optional[int] = ..., webhookId: _Optional[int] = ..., request: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., response: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., status: _Optional[str] = ..., createdDate: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updatedDate: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., assistantId: _Optional[int] = ..., projectId: _Optional[int] = ..., organizationId: _Optional[int] = ..., assistantConversationId: _Optional[int] = ..., assetPrefix: _Optional[str] = ..., event: _Optional[str] = ..., responseStatus: _Optional[int] = ..., timeTaken: _Optional[int] = ..., retryCount: _Optional[int] = ..., httpMethod: _Optional[str] = ..., httpUrl: _Optional[str] = ...) -> None: ...

class CreateAssistantWebhookRequest(_message.Message):
    __slots__ = ("assistantEvents", "description", "options", "assistantId", "executionPriority")
    ASSISTANTEVENTS_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    OPTIONS_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTID_FIELD_NUMBER: _ClassVar[int]
    EXECUTIONPRIORITY_FIELD_NUMBER: _ClassVar[int]
    assistantEvents: _containers.RepeatedScalarFieldContainer[str]
    description: str
    options: _containers.RepeatedCompositeFieldContainer[_common_pb2.Metadata]
    assistantId: int
    executionPriority: int
    def __init__(self, assistantEvents: _Optional[_Iterable[str]] = ..., description: _Optional[str] = ..., options: _Optional[_Iterable[_Union[_common_pb2.Metadata, _Mapping]]] = ..., assistantId: _Optional[int] = ..., executionPriority: _Optional[int] = ...) -> None: ...

class UpdateAssistantWebhookRequest(_message.Message):
    __slots__ = ("id", "assistantEvents", "description", "options", "assistantId", "executionPriority")
    ID_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTEVENTS_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    OPTIONS_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTID_FIELD_NUMBER: _ClassVar[int]
    EXECUTIONPRIORITY_FIELD_NUMBER: _ClassVar[int]
    id: int
    assistantEvents: _containers.RepeatedScalarFieldContainer[str]
    description: str
    options: _containers.RepeatedCompositeFieldContainer[_common_pb2.Metadata]
    assistantId: int
    executionPriority: int
    def __init__(self, id: _Optional[int] = ..., assistantEvents: _Optional[_Iterable[str]] = ..., description: _Optional[str] = ..., options: _Optional[_Iterable[_Union[_common_pb2.Metadata, _Mapping]]] = ..., assistantId: _Optional[int] = ..., executionPriority: _Optional[int] = ...) -> None: ...

class GetAssistantWebhookRequest(_message.Message):
    __slots__ = ("id", "assistantId")
    ID_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTID_FIELD_NUMBER: _ClassVar[int]
    id: int
    assistantId: int
    def __init__(self, id: _Optional[int] = ..., assistantId: _Optional[int] = ...) -> None: ...

class DeleteAssistantWebhookRequest(_message.Message):
    __slots__ = ("id", "assistantId")
    ID_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTID_FIELD_NUMBER: _ClassVar[int]
    id: int
    assistantId: int
    def __init__(self, id: _Optional[int] = ..., assistantId: _Optional[int] = ...) -> None: ...

class GetAssistantWebhookResponse(_message.Message):
    __slots__ = ("code", "success", "data", "error")
    CODE_FIELD_NUMBER: _ClassVar[int]
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    code: int
    success: bool
    data: AssistantWebhook
    error: _common_pb2.Error
    def __init__(self, code: _Optional[int] = ..., success: bool = ..., data: _Optional[_Union[AssistantWebhook, _Mapping]] = ..., error: _Optional[_Union[_common_pb2.Error, _Mapping]] = ...) -> None: ...

class GetAllAssistantWebhookRequest(_message.Message):
    __slots__ = ("webhookId", "assistantId", "paginate", "criterias")
    WEBHOOKID_FIELD_NUMBER: _ClassVar[int]
    ASSISTANTID_FIELD_NUMBER: _ClassVar[int]
    PAGINATE_FIELD_NUMBER: _ClassVar[int]
    CRITERIAS_FIELD_NUMBER: _ClassVar[int]
    webhookId: int
    assistantId: int
    paginate: _common_pb2.Paginate
    criterias: _containers.RepeatedCompositeFieldContainer[_common_pb2.Criteria]
    def __init__(self, webhookId: _Optional[int] = ..., assistantId: _Optional[int] = ..., paginate: _Optional[_Union[_common_pb2.Paginate, _Mapping]] = ..., criterias: _Optional[_Iterable[_Union[_common_pb2.Criteria, _Mapping]]] = ...) -> None: ...

class GetAllAssistantWebhookResponse(_message.Message):
    __slots__ = ("code", "success", "data", "error", "paginated")
    CODE_FIELD_NUMBER: _ClassVar[int]
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    PAGINATED_FIELD_NUMBER: _ClassVar[int]
    code: int
    success: bool
    data: _containers.RepeatedCompositeFieldContainer[AssistantWebhook]
    error: _common_pb2.Error
    paginated: _common_pb2.Paginated
    def __init__(self, code: _Optional[int] = ..., success: bool = ..., data: _Optional[_Iterable[_Union[AssistantWebhook, _Mapping]]] = ..., error: _Optional[_Union[_common_pb2.Error, _Mapping]] = ..., paginated: _Optional[_Union[_common_pb2.Paginated, _Mapping]] = ...) -> None: ...

class GetAllAssistantWebhookLogRequest(_message.Message):
    __slots__ = ("projectId", "paginate", "criterias", "order")
    PROJECTID_FIELD_NUMBER: _ClassVar[int]
    PAGINATE_FIELD_NUMBER: _ClassVar[int]
    CRITERIAS_FIELD_NUMBER: _ClassVar[int]
    ORDER_FIELD_NUMBER: _ClassVar[int]
    projectId: int
    paginate: _common_pb2.Paginate
    criterias: _containers.RepeatedCompositeFieldContainer[_common_pb2.Criteria]
    order: _common_pb2.Ordering
    def __init__(self, projectId: _Optional[int] = ..., paginate: _Optional[_Union[_common_pb2.Paginate, _Mapping]] = ..., criterias: _Optional[_Iterable[_Union[_common_pb2.Criteria, _Mapping]]] = ..., order: _Optional[_Union[_common_pb2.Ordering, _Mapping]] = ...) -> None: ...

class GetAssistantWebhookLogRequest(_message.Message):
    __slots__ = ("projectId", "id")
    PROJECTID_FIELD_NUMBER: _ClassVar[int]
    ID_FIELD_NUMBER: _ClassVar[int]
    projectId: int
    id: int
    def __init__(self, projectId: _Optional[int] = ..., id: _Optional[int] = ...) -> None: ...

class GetAssistantWebhookLogResponse(_message.Message):
    __slots__ = ("code", "success", "data", "error")
    CODE_FIELD_NUMBER: _ClassVar[int]
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    code: int
    success: bool
    data: AssistantWebhookLog
    error: _common_pb2.Error
    def __init__(self, code: _Optional[int] = ..., success: bool = ..., data: _Optional[_Union[AssistantWebhookLog, _Mapping]] = ..., error: _Optional[_Union[_common_pb2.Error, _Mapping]] = ...) -> None: ...

class GetAllAssistantWebhookLogResponse(_message.Message):
    __slots__ = ("code", "success", "data", "error", "paginated")
    CODE_FIELD_NUMBER: _ClassVar[int]
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    PAGINATED_FIELD_NUMBER: _ClassVar[int]
    code: int
    success: bool
    data: _containers.RepeatedCompositeFieldContainer[AssistantWebhookLog]
    error: _common_pb2.Error
    paginated: _common_pb2.Paginated
    def __init__(self, code: _Optional[int] = ..., success: bool = ..., data: _Optional[_Iterable[_Union[AssistantWebhookLog, _Mapping]]] = ..., error: _Optional[_Union[_common_pb2.Error, _Mapping]] = ..., paginated: _Optional[_Union[_common_pb2.Paginated, _Mapping]] = ...) -> None: ...
