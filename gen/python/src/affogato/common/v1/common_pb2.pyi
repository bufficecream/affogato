import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ChunkRef(_message.Message):
    __slots__ = ("video_id", "chunk_seq")
    VIDEO_ID_FIELD_NUMBER: _ClassVar[int]
    CHUNK_SEQ_FIELD_NUMBER: _ClassVar[int]
    video_id: str
    chunk_seq: int
    def __init__(self, video_id: _Optional[str] = ..., chunk_seq: _Optional[int] = ...) -> None: ...

class ObjectRef(_message.Message):
    __slots__ = ("bucket", "key", "size_bytes", "content_type", "sha256_hex", "etag")
    BUCKET_FIELD_NUMBER: _ClassVar[int]
    KEY_FIELD_NUMBER: _ClassVar[int]
    SIZE_BYTES_FIELD_NUMBER: _ClassVar[int]
    CONTENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    SHA256_HEX_FIELD_NUMBER: _ClassVar[int]
    ETAG_FIELD_NUMBER: _ClassVar[int]
    bucket: str
    key: str
    size_bytes: int
    content_type: str
    sha256_hex: str
    etag: str
    def __init__(self, bucket: _Optional[str] = ..., key: _Optional[str] = ..., size_bytes: _Optional[int] = ..., content_type: _Optional[str] = ..., sha256_hex: _Optional[str] = ..., etag: _Optional[str] = ...) -> None: ...

class TimeRange(_message.Message):
    __slots__ = ("start_ms", "end_ms")
    START_MS_FIELD_NUMBER: _ClassVar[int]
    END_MS_FIELD_NUMBER: _ClassVar[int]
    start_ms: int
    end_ms: int
    def __init__(self, start_ms: _Optional[int] = ..., end_ms: _Optional[int] = ...) -> None: ...

class MediaInfo(_message.Message):
    __slots__ = ("container", "video_codec", "audio_codec", "width", "height", "frame_rate_num", "frame_rate_den", "has_audio", "audio_sample_rate_hz", "audio_channels", "bitrate_bps")
    CONTAINER_FIELD_NUMBER: _ClassVar[int]
    VIDEO_CODEC_FIELD_NUMBER: _ClassVar[int]
    AUDIO_CODEC_FIELD_NUMBER: _ClassVar[int]
    WIDTH_FIELD_NUMBER: _ClassVar[int]
    HEIGHT_FIELD_NUMBER: _ClassVar[int]
    FRAME_RATE_NUM_FIELD_NUMBER: _ClassVar[int]
    FRAME_RATE_DEN_FIELD_NUMBER: _ClassVar[int]
    HAS_AUDIO_FIELD_NUMBER: _ClassVar[int]
    AUDIO_SAMPLE_RATE_HZ_FIELD_NUMBER: _ClassVar[int]
    AUDIO_CHANNELS_FIELD_NUMBER: _ClassVar[int]
    BITRATE_BPS_FIELD_NUMBER: _ClassVar[int]
    container: str
    video_codec: str
    audio_codec: str
    width: int
    height: int
    frame_rate_num: int
    frame_rate_den: int
    has_audio: bool
    audio_sample_rate_hz: int
    audio_channels: int
    bitrate_bps: int
    def __init__(self, container: _Optional[str] = ..., video_codec: _Optional[str] = ..., audio_codec: _Optional[str] = ..., width: _Optional[int] = ..., height: _Optional[int] = ..., frame_rate_num: _Optional[int] = ..., frame_rate_den: _Optional[int] = ..., has_audio: _Optional[bool] = ..., audio_sample_rate_hz: _Optional[int] = ..., audio_channels: _Optional[int] = ..., bitrate_bps: _Optional[int] = ...) -> None: ...

class EventMeta(_message.Message):
    __slots__ = ("event_id", "occurred_at", "producer", "producer_version", "causation_id")
    EVENT_ID_FIELD_NUMBER: _ClassVar[int]
    OCCURRED_AT_FIELD_NUMBER: _ClassVar[int]
    PRODUCER_FIELD_NUMBER: _ClassVar[int]
    PRODUCER_VERSION_FIELD_NUMBER: _ClassVar[int]
    CAUSATION_ID_FIELD_NUMBER: _ClassVar[int]
    event_id: str
    occurred_at: _timestamp_pb2.Timestamp
    producer: str
    producer_version: str
    causation_id: str
    def __init__(self, event_id: _Optional[str] = ..., occurred_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., producer: _Optional[str] = ..., producer_version: _Optional[str] = ..., causation_id: _Optional[str] = ...) -> None: ...
