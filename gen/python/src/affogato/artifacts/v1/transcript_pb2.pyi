from affogato.common.v1 import common_pb2 as _common_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Transcript(_message.Message):
    __slots__ = ("chunk", "media_time", "language", "stt_model", "text", "segments")
    CHUNK_FIELD_NUMBER: _ClassVar[int]
    MEDIA_TIME_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    STT_MODEL_FIELD_NUMBER: _ClassVar[int]
    TEXT_FIELD_NUMBER: _ClassVar[int]
    SEGMENTS_FIELD_NUMBER: _ClassVar[int]
    chunk: _common_pb2.ChunkRef
    media_time: _common_pb2.TimeRange
    language: str
    stt_model: str
    text: str
    segments: _containers.RepeatedCompositeFieldContainer[TranscriptSegment]
    def __init__(self, chunk: _Optional[_Union[_common_pb2.ChunkRef, _Mapping]] = ..., media_time: _Optional[_Union[_common_pb2.TimeRange, _Mapping]] = ..., language: _Optional[str] = ..., stt_model: _Optional[str] = ..., text: _Optional[str] = ..., segments: _Optional[_Iterable[_Union[TranscriptSegment, _Mapping]]] = ...) -> None: ...

class TranscriptSegment(_message.Message):
    __slots__ = ("media_time", "text", "avg_logprob")
    MEDIA_TIME_FIELD_NUMBER: _ClassVar[int]
    TEXT_FIELD_NUMBER: _ClassVar[int]
    AVG_LOGPROB_FIELD_NUMBER: _ClassVar[int]
    media_time: _common_pb2.TimeRange
    text: str
    avg_logprob: float
    def __init__(self, media_time: _Optional[_Union[_common_pb2.TimeRange, _Mapping]] = ..., text: _Optional[str] = ..., avg_logprob: _Optional[float] = ...) -> None: ...
