package helper

import (
	"fmt"
	"sort"
)

// A Session edit actions
type Session struct {
	actions []contentAction
	newSize int

	// original oldContent
	oldContent []byte
}

func NewSession(content []byte) *Session {
	return &Session{oldContent: content, newSize: len(content)}
}

type contentAction struct {
	begin   int
	end     int
	content string
}

func (b *Session) Add(pos int, new string) {
	if pos < 0 || pos > len(b.oldContent) {
		panic("pos invalid")
	}
	b.newSize += len(new)
	b.actions = append(b.actions, contentAction{pos, pos, new})
}

func (b *Session) Remove(begin, end int) {
	if end == -1 {
		end = len(b.oldContent)
	}
	if end < begin || begin < 0 || end > len(b.oldContent) {
		panic("pos invalid")
	}
	b.newSize -= end - begin
	b.actions = append(b.actions, contentAction{begin, end, ""})
}

func (b *Session) Substitute(start, end int, new string) {
	if end == -1 {
		end = len(b.oldContent)
	}
	if end < start || start < 0 || end > len(b.oldContent) {
		panic("pos invalid")
	}
	b.newSize += len(new) - (end - start)
	b.actions = append(b.actions, contentAction{start, end, new})
}

func (b *Session) String() string {
	return string(b.ApplyActions())
}

// ApplyActions apply actions to generate new content
func (b *Session) ApplyActions() []byte {
	// sort actions so that we can apply
	// action in position ascending order
	sort.Slice(b.actions, func(i, j int) bool {
		d := b.actions[i].begin - b.actions[j].begin
		if d != 0 {
			return d < 0
		}
		return b.actions[i].end < b.actions[j].end
	})

	newContent := make([]byte, 0, b.newSize)
	off := 0
	for i, action := range b.actions {
		if action.begin < off {
			panic(fmt.Sprintf("failed to apply actions:%+v", b.actions[i-1]))
		}
		newContent = append(newContent, b.oldContent[off:action.begin]...)
		newContent = append(newContent, action.content...)
		off = action.end
	}
	newContent = append(newContent, b.oldContent[off:]...)
	return newContent
}
