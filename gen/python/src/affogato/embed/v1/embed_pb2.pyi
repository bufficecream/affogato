from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Purpose(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    PURPOSE_UNSPECIFIED: _ClassVar[Purpose]
    PURPOSE_QUERY: _ClassVar[Purpose]
    PURPOSE_PASSAGE: _ClassVar[Purpose]
PURPOSE_UNSPECIFIED: Purpose
PURPOSE_QUERY: Purpose
PURPOSE_PASSAGE: Purpose

class EmbedRequest(_message.Message):
    __slots__ = ("texts", "purpose")
    TEXTS_FIELD_NUMBER: _ClassVar[int]
    PURPOSE_FIELD_NUMBER: _ClassVar[int]
    texts: _containers.RepeatedScalarFieldContainer[str]
    purpose: Purpose
    def __init__(self, texts: _Optional[_Iterable[str]] = ..., purpose: _Optional[_Union[Purpose, str]] = ...) -> None: ...

class EmbedResponse(_message.Message):
    __slots__ = ("model", "dim", "vectors")
    MODEL_FIELD_NUMBER: _ClassVar[int]
    DIM_FIELD_NUMBER: _ClassVar[int]
    VECTORS_FIELD_NUMBER: _ClassVar[int]
    model: str
    dim: int
    vectors: _containers.RepeatedCompositeFieldContainer[Vector]
    def __init__(self, model: _Optional[str] = ..., dim: _Optional[int] = ..., vectors: _Optional[_Iterable[_Union[Vector, _Mapping]]] = ...) -> None: ...

class Vector(_message.Message):
    __slots__ = ("values",)
    VALUES_FIELD_NUMBER: _ClassVar[int]
    values: _containers.RepeatedScalarFieldContainer[float]
    def __init__(self, values: _Optional[_Iterable[float]] = ...) -> None: ...
