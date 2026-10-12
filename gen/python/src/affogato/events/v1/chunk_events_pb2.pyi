import datetime

from affogato.common.v1 import common_pb2 as _common_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class IndexOutcome(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    INDEX_OUTCOME_UNSPECIFIED: _ClassVar[IndexOutcome]
    INDEX_OUTCOME_INDEXED: _ClassVar[IndexOutcome]
    INDEX_OUTCOME_NO_SPEECH: _ClassVar[IndexOutcome]
    INDEX_OUTCOME_NO_AUDIO: _ClassVar[IndexOutcome]
INDEX_OUTCOME_UNSPECIFIED: IndexOutcome
INDEX_OUTCOME_INDEXED: IndexOutcome
INDEX_OUTCOME_NO_SPEECH: IndexOutcome
INDEX_OUTCOME_NO_AUDIO: IndexOutcome

class VideoChunkUploaded(_message.Message):
    __slots__ = ("meta", "chunk", "raw_object", "media_time", "media", "source_id", "captured_at")
    META_FIELD_NUMBER: _ClassVar[int]
    CHUNK_FIELD_NUMBER: _ClassVar[int]
    RAW_OBJECT_FIELD_NUMBER: _ClassVar[int]
    MEDIA_TIME_FIELD_NUMBER: _ClassVar[int]
    MEDIA_FIELD_NUMBER: _ClassVar[int]
    SOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    CAPTURED_AT_FIELD_NUMBER: _ClassVar[int]
    meta: _common_pb2.EventMeta
    chunk: _common_pb2.ChunkRef
    raw_object: _common_pb2.ObjectRef
    media_time: _common_pb2.TimeRange
    media: _common_pb2.MediaInfo
    source_id: str
    captured_at: _timestamp_pb2.Timestamp
    def __init__(self, meta: _Optional[_Union[_common_pb2.EventMeta, _Mapping]] = ..., chunk: _Optional[_Union[_common_pb2.ChunkRef, _Mapping]] = ..., raw_object: _Optional[_Union[_common_pb2.ObjectRef, _Mapping]] = ..., media_time: _Optional[_Union[_common_pb2.TimeRange, _Mapping]] = ..., media: _Optional[_Union[_common_pb2.MediaInfo, _Mapping]] = ..., source_id: _Optional[str] = ..., captured_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ChunkHlsReady(_message.Message):
    __slots__ = ("meta", "chunk", "rendition", "segment", "thumbnail", "media_time", "processing_ms")
    META_FIELD_NUMBER: _ClassVar[int]
    CHUNK_FIELD_NUMBER: _ClassVar[int]
    RENDITION_FIELD_NUMBER: _ClassVar[int]
    SEGMENT_FIELD_NUMBER: _ClassVar[int]
    THUMBNAIL_FIELD_NUMBER: _ClassVar[int]
    MEDIA_TIME_FIELD_NUMBER: _ClassVar[int]
    PROCESSING_MS_FIELD_NUMBER: _ClassVar[int]
    meta: _common_pb2.EventMeta
    chunk: _common_pb2.ChunkRef
    rendition: str
    segment: _common_pb2.ObjectRef
    thumbnail: _common_pb2.ObjectRef
    media_time: _common_pb2.TimeRange
    processing_ms: int
    def __init__(self, meta: _Optional[_Union[_common_pb2.EventMeta, _Mapping]] = ..., chunk: _Optional[_Union[_common_pb2.ChunkRef, _Mapping]] = ..., rendition: _Optional[str] = ..., segment: _Optional[_Union[_common_pb2.ObjectRef, _Mapping]] = ..., thumbnail: _Optional[_Union[_common_pb2.ObjectRef, _Mapping]] = ..., media_time: _Optional[_Union[_common_pb2.TimeRange, _Mapping]] = ..., processing_ms: _Optional[int] = ...) -> None: ...

class ChunkTranscriptIndexed(_message.Message):
    __slots__ = ("meta", "chunk", "media_time", "outcome", "transcript", "language", "stt_model", "embedding_model", "qdrant_collection", "point_id", "processing_ms", "transcript_chars")
    META_FIELD_NUMBER: _ClassVar[int]
    CHUNK_FIELD_NUMBER: _ClassVar[int]
    MEDIA_TIME_FIELD_NUMBER: _ClassVar[int]
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    TRANSCRIPT_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    STT_MODEL_FIELD_NUMBER: _ClassVar[int]
    EMBEDDING_MODEL_FIELD_NUMBER: _ClassVar[int]
    QDRANT_COLLECTION_FIELD_NUMBER: _ClassVar[int]
    POINT_ID_FIELD_NUMBER: _ClassVar[int]
    PROCESSING_MS_FIELD_NUMBER: _ClassVar[int]
    TRANSCRIPT_CHARS_FIELD_NUMBER: _ClassVar[int]
    meta: _common_pb2.EventMeta
    chunk: _common_pb2.ChunkRef
    media_time: _common_pb2.TimeRange
    outcome: IndexOutcome
    transcript: _common_pb2.ObjectRef
    language: str
    stt_model: str
    embedding_model: str
    qdrant_collection: str
    point_id: str
    processing_ms: int
    transcript_chars: int
    def __init__(self, meta: _Optional[_Union[_common_pb2.EventMeta, _Mapping]] = ..., chunk: _Optional[_Union[_common_pb2.ChunkRef, _Mapping]] = ..., media_time: _Optional[_Union[_common_pb2.TimeRange, _Mapping]] = ..., outcome: _Optional[_Union[IndexOutcome, str]] = ..., transcript: _Optional[_Union[_common_pb2.ObjectRef, _Mapping]] = ..., language: _Optional[str] = ..., stt_model: _Optional[str] = ..., embedding_model: _Optional[str] = ..., qdrant_collection: _Optional[str] = ..., point_id: _Optional[str] = ..., processing_ms: _Optional[int] = ..., transcript_chars: _Optional[int] = ...) -> None: ...
