from affogato.common.v1 import common_pb2 as _common_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class VideoIngestCompleted(_message.Message):
    __slots__ = ("meta", "video_id", "chunk_count", "media_time")
    META_FIELD_NUMBER: _ClassVar[int]
    VIDEO_ID_FIELD_NUMBER: _ClassVar[int]
    CHUNK_COUNT_FIELD_NUMBER: _ClassVar[int]
    MEDIA_TIME_FIELD_NUMBER: _ClassVar[int]
    meta: _common_pb2.EventMeta
    video_id: str
    chunk_count: int
    media_time: _common_pb2.TimeRange
    def __init__(self, meta: _Optional[_Union[_common_pb2.EventMeta, _Mapping]] = ..., video_id: _Optional[str] = ..., chunk_count: _Optional[int] = ..., media_time: _Optional[_Union[_common_pb2.TimeRange, _Mapping]] = ...) -> None: ...
