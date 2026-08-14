package numeric

import "errors"

// ErrNumericClosed 表示 NumericComponent 已销毁。
var ErrNumericClosed = errors.New("numeric: component closed")
