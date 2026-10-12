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

class ErrorClass(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ERROR_CLASS_UNSPECIFIED: _ClassVar[ErrorClass]
    ERROR_CLASS_INVALID_MEDIA: _ClassVar[ErrorClass]
    ERROR_CLASS_OBJECT_NOT_FOUND: _ClassVar[ErrorClass]
    ERROR_CLASS_CHECKSUM_MISMATCH: _ClassVar[ErrorClass]
    ERROR_CLASS_SCHEMA: _ClassVar[ErrorClass]
    ERROR_CLASS_TIMEOUT: _ClassVar[ErrorClass]
    ERROR_CLASS_RETRIES_EXHAUSTED: _ClassVar[ErrorClass]
    ERROR_CLASS_INTERNAL: _ClassVar[ErrorClass]
ERROR_CLASS_UNSPECIFIED: ErrorClass
ERROR_CLASS_INVALID_MEDIA: ErrorClass
ERROR_CLASS_OBJECT_NOT_FOUND: ErrorClass
ERROR_CLASS_CHECKSUM_MISMATCH: ErrorClass
ERROR_CLASS_SCHEMA: ErrorClass
ERROR_CLASS_TIMEOUT: ErrorClass
ERROR_CLASS_RETRIES_EXHAUSTED: ErrorClass
ERROR_CLASS_INTERNAL: ErrorClass

class Header(_message.Message):
    __slots__ = ("name", "values")
    NAME_FIELD_NUMBER: _ClassVar[int]
    VALUES_FIELD_NUMBER: _ClassVar[int]
    name: str
    values: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, name: _Optional[str] = ..., values: _Optional[_Iterable[str]] = ...) -> None: ...

class DeadLetter(_message.Message):
    __slots__ = ("meta", "original_stream", "original_stream_seq", "original_subject", "original_headers", "original_payload", "original_published_at", "consumer", "delivery_count", "error_class", "error_code", "error_message", "redrivable", "chunk", "worker_instance")
    META_FIELD_NUMBER: _ClassVar[int]
    ORIGINAL_STREAM_FIELD_NUMBER: _ClassVar[int]
    ORIGINAL_STREAM_SEQ_FIELD_NUMBER: _ClassVar[int]
    ORIGINAL_SUBJECT_FIELD_NUMBER: _ClassVar[int]
    ORIGINAL_HEADERS_FIELD_NUMBER: _ClassVar[int]
    ORIGINAL_PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    ORIGINAL_PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    CONSUMER_FIELD_NUMBER: _ClassVar[int]
    DELIVERY_COUNT_FIELD_NUMBER: _ClassVar[int]
    ERROR_CLASS_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    REDRIVABLE_FIELD_NUMBER: _ClassVar[int]
    CHUNK_FIELD_NUMBER: _ClassVar[int]
    WORKER_INSTANCE_FIELD_NUMBER: _ClassVar[int]
    meta: _common_pb2.EventMeta
    original_stream: str
    original_stream_seq: int
    original_subject: str
    original_headers: _containers.RepeatedCompositeFieldContainer[Header]
    original_payload: bytes
    original_published_at: _timestamp_pb2.Timestamp
    consumer: str
    delivery_count: int
    error_class: ErrorClass
    error_code: str
    error_message: str
    redrivable: bool
    chunk: _common_pb2.ChunkRef
    worker_instance: str
    def __init__(self, meta: _Optional[_Union[_common_pb2.EventMeta, _Mapping]] = ..., original_stream: _Optional[str] = ..., original_stream_seq: _Optional[int] = ..., original_subject: _Optional[str] = ..., original_headers: _Optional[_Iterable[_Union[Header, _Mapping]]] = ..., original_payload: _Optional[bytes] = ..., original_published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., consumer: _Optional[str] = ..., delivery_count: _Optional[int] = ..., error_class: _Optional[_Union[ErrorClass, str]] = ..., error_code: _Optional[str] = ..., error_message: _Optional[str] = ..., redrivable: _Optional[bool] = ..., chunk: _Optional[_Union[_common_pb2.ChunkRef, _Mapping]] = ..., worker_instance: _Optional[str] = ...) -> None: ...
