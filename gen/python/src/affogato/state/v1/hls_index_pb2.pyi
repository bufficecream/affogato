import datetime

from affogato.common.v1 import common_pb2 as _common_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class FinalizeReason(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    FINALIZE_REASON_UNSPECIFIED: _ClassVar[FinalizeReason]
    FINALIZE_REASON_COMPLETED: _ClassVar[FinalizeReason]
    FINALIZE_REASON_IDLE_TIMEOUT: _ClassVar[FinalizeReason]

class SegmentStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SEGMENT_STATUS_UNSPECIFIED: _ClassVar[SegmentStatus]
    SEGMENT_STATUS_READY: _ClassVar[SegmentStatus]
    SEGMENT_STATUS_FAILED: _ClassVar[SegmentStatus]
FINALIZE_REASON_UNSPECIFIED: FinalizeReason
FINALIZE_REASON_COMPLETED: FinalizeReason
FINALIZE_REASON_IDLE_TIMEOUT: FinalizeReason
SEGMENT_STATUS_UNSPECIFIED: SegmentStatus
SEGMENT_STATUS_READY: SegmentStatus
SEGMENT_STATUS_FAILED: SegmentStatus

class HlsIndex(_message.Message):
    __slots__ = ("video_id", "rendition", "segments", "expected_chunk_count", "finalized", "last_activity_at", "finalize_reason")
    VIDEO_ID_FIELD_NUMBER: _ClassVar[int]
    RENDITION_FIELD_NUMBER: _ClassVar[int]
    SEGMENTS_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_CHUNK_COUNT_FIELD_NUMBER: _ClassVar[int]
    FINALIZED_FIELD_NUMBER: _ClassVar[int]
    LAST_ACTIVITY_AT_FIELD_NUMBER: _ClassVar[int]
    FINALIZE_REASON_FIELD_NUMBER: _ClassVar[int]
    video_id: str
    rendition: str
    segments: _containers.RepeatedCompositeFieldContainer[Segment]
    expected_chunk_count: int
    finalized: bool
    last_activity_at: _timestamp_pb2.Timestamp
    finalize_reason: FinalizeReason
    def __init__(self, video_id: _Optional[str] = ..., rendition: _Optional[str] = ..., segments: _Optional[_Iterable[_Union[Segment, _Mapping]]] = ..., expected_chunk_count: _Optional[int] = ..., finalized: _Optional[bool] = ..., last_activity_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., finalize_reason: _Optional[_Union[FinalizeReason, str]] = ...) -> None: ...

class Segment(_message.Message):
    __slots__ = ("chunk_seq", "media_time", "status")
    CHUNK_SEQ_FIELD_NUMBER: _ClassVar[int]
    MEDIA_TIME_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    chunk_seq: int
    media_time: _common_pb2.TimeRange
    status: SegmentStatus
    def __init__(self, chunk_seq: _Optional[int] = ..., media_time: _Optional[_Union[_common_pb2.TimeRange, _Mapping]] = ..., status: _Optional[_Union[SegmentStatus, str]] = ...) -> None: ...
