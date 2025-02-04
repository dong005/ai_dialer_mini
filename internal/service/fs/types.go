package fs

// CallStatus 通话状态
type CallStatus int

const (
	CallStatusNew      CallStatus = iota // 新建
	CallStatusRinging                    // 振铃
	CallStatusAnswered                   // 已接听
	CallStatusHangup                     // 挂断
)

// String 返回通话状态的字符串表示
func (s CallStatus) String() string {
	switch s {
	case CallStatusNew:
		return "新建"
	case CallStatusRinging:
		return "振铃"
	case CallStatusAnswered:
		return "已接听"
	case CallStatusHangup:
		return "挂断"
	default:
		return "未知"
	}
}
