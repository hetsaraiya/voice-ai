import app.bridges.artifacts.protos.common_pb2 as _common_pb2
import app.bridges.artifacts.protos.talk_api_pb2 as _talk_api_pb2
import app.bridges.artifacts.protos.observability_api_pb2 as _observability_api_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class TalkInput(_message.Message):
    __slots__ = ("initialization", "configuration", "user", "interruption", "metadata", "metric", "toolCall", "toolCallResult", "assistant")
    INITIALIZATION_FIELD_NUMBER: _ClassVar[int]
    CONFIGURATION_FIELD_NUMBER: _ClassVar[int]
    USER_FIELD_NUMBER: _ClassVar[int]
    INTERRUPTION_FIELD_NUMBER: _ClassVar[int]
    METADATA_FIELD_NUMBER: _ClassVar[int]
    METRIC_FIELD_NUMBER: _ClassVar[int]
    TOOLCALL_FIELD_NUMBER: _ClassVar[int]
    TOOLCALLRESULT_FIELD_NUMBER: _ClassVar[int]
    ASSISTANT_FIELD_NUMBER: _ClassVar[int]
    initialization: _talk_api_pb2.ConversationInitialization
    configuration: _talk_api_pb2.ConversationConfiguration
    user: _talk_api_pb2.ConversationUserMessage
    interruption: _talk_api_pb2.ConversationInterruption
    metadata: _talk_api_pb2.ConversationMetadata
    metric: _talk_api_pb2.ConversationMetric
    toolCall: _talk_api_pb2.ConversationToolCall
    toolCallResult: _talk_api_pb2.ConversationToolCallResult
    assistant: _talk_api_pb2.ConversationAssistantMessage
    def __init__(self, initialization: _Optional[_Union[_talk_api_pb2.ConversationInitialization, _Mapping]] = ..., configuration: _Optional[_Union[_talk_api_pb2.ConversationConfiguration, _Mapping]] = ..., user: _Optional[_Union[_talk_api_pb2.ConversationUserMessage, _Mapping]] = ..., interruption: _Optional[_Union[_talk_api_pb2.ConversationInterruption, _Mapping]] = ..., metadata: _Optional[_Union[_talk_api_pb2.ConversationMetadata, _Mapping]] = ..., metric: _Optional[_Union[_talk_api_pb2.ConversationMetric, _Mapping]] = ..., toolCall: _Optional[_Union[_talk_api_pb2.ConversationToolCall, _Mapping]] = ..., toolCallResult: _Optional[_Union[_talk_api_pb2.ConversationToolCallResult, _Mapping]] = ..., assistant: _Optional[_Union[_talk_api_pb2.ConversationAssistantMessage, _Mapping]] = ...) -> None: ...

class TalkOutput(_message.Message):
    __slots__ = ("code", "success", "initialization", "interruption", "user", "assistant", "toolCall", "toolCallResult", "error", "observability")
    CODE_FIELD_NUMBER: _ClassVar[int]
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    INITIALIZATION_FIELD_NUMBER: _ClassVar[int]
    INTERRUPTION_FIELD_NUMBER: _ClassVar[int]
    USER_FIELD_NUMBER: _ClassVar[int]
    ASSISTANT_FIELD_NUMBER: _ClassVar[int]
    TOOLCALL_FIELD_NUMBER: _ClassVar[int]
    TOOLCALLRESULT_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    OBSERVABILITY_FIELD_NUMBER: _ClassVar[int]
    code: int
    success: bool
    initialization: _talk_api_pb2.ConversationInitialization
    interruption: _talk_api_pb2.ConversationInterruption
    user: _talk_api_pb2.ConversationUserMessage
    assistant: _talk_api_pb2.ConversationAssistantMessage
    toolCall: _talk_api_pb2.ConversationToolCall
    toolCallResult: _talk_api_pb2.ConversationToolCallResult
    error: _common_pb2.Error
    observability: _observability_api_pb2.ObservabilityRecord
    def __init__(self, code: _Optional[int] = ..., success: bool = ..., initialization: _Optional[_Union[_talk_api_pb2.ConversationInitialization, _Mapping]] = ..., interruption: _Optional[_Union[_talk_api_pb2.ConversationInterruption, _Mapping]] = ..., user: _Optional[_Union[_talk_api_pb2.ConversationUserMessage, _Mapping]] = ..., assistant: _Optional[_Union[_talk_api_pb2.ConversationAssistantMessage, _Mapping]] = ..., toolCall: _Optional[_Union[_talk_api_pb2.ConversationToolCall, _Mapping]] = ..., toolCallResult: _Optional[_Union[_talk_api_pb2.ConversationToolCallResult, _Mapping]] = ..., error: _Optional[_Union[_common_pb2.Error, _Mapping]] = ..., observability: _Optional[_Union[_observability_api_pb2.ObservabilityRecord, _Mapping]] = ...) -> None: ...
