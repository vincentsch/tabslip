package apis

import (
	"bufio"
	"bytes"
	"io"

	"github.com/vincentsch/tabslip/internal/receipt"
)

type Capture struct {
	Receipt   *receipt.Receipt
	Done      bool
	Keep      bool
	Text      []byte
	Truncated bool
}

func (c *Capture) appendText(s string) {
	if !c.Keep {
		return
	}
	n := Limit - len(c.Text)
	if len(s) > n {
		s = s[:n]
		c.Truncated = true
	}
	c.Text = append(c.Text, s...)
}
func (c *Capture) value(b []byte, stream bool) bool {
	v, ok := Object(b)
	if !ok {
		Unavailable(c.Receipt)
		return false
	}
	if c.Receipt.Request.API == "anthropic.messages" {
		c.messages(v, stream)
		return true
	}
	if c.Receipt.Request.API == "openai.responses" {
		c.responses(v, stream)
		return true
	}
	if c.Receipt.Request.API == "openai.embeddings" {
		c.embeddings(v)
		return true
	}
	c.chat(v, stream)
	return true
}
func (c *Capture) Parse(src io.Reader, stream bool) {
	if !stream {
		b, err := io.ReadAll(io.LimitReader(src, Limit+1))
		if c.Keep {
			n := len(b)
			if n > Limit {
				n = Limit
				c.Truncated = true
			}
			c.Text = append(c.Text, b[:n]...)
		}
		if err != nil || len(b) > Limit {
			Unavailable(c.Receipt)
			return
		}
		c.value(b, false)
		return
	}
	reader := bufio.NewReaderSize(src, 32*1024)
	var event []byte
	size := 0
	overflow := false
	metadata := true
	unavailable := func() {
		Unavailable(c.Receipt)
		metadata = false
		if c.Keep {
			c.Truncated = true
		}
	}
	dispatch := func() {
		if overflow || len(event) == 0 {
			return
		}
		b := bytes.TrimSpace(event)
		if bytes.Equal(b, []byte("[DONE]")) {
			if c.Receipt.Request.API == "" || c.Receipt.Request.API == "openai.chat" {
				c.Done = true
			}
			return
		}
		if metadata {
			if !c.value(b, true) {
				unavailable()
			}
		} else {
			if v, ok := Object(b); ok {
				// Framing remains observable even when field extraction has stopped.
				switch c.Receipt.Request.API {
				case "openai.responses":
					switch v["type"] {
					case "response.completed", "response.incomplete", "response.failed", "error":
						c.Done = true
					}
				case "anthropic.messages":
					if v["type"] == "message_stop" || (v["type"] == "error" && object(v["error"]) != nil) {
						c.Done = true
					}
				}
				if c.Keep {
					c.content(v)
				}
			}
		}
	}
	for {
		var full []byte
		var err error
		var part []byte
		lineBytes := 0
		for {
			part, err = reader.ReadSlice('\n')
			size += len(part)
			lineBytes += len(part)
			if size > Limit {
				if !overflow {
					unavailable()
				}
				overflow = true
				event = nil
				full = nil
			}
			if !overflow {
				full = append(full, part...)
			}
			if err != bufio.ErrBufferFull {
				break
			}
		}
		full = bytes.TrimSuffix(bytes.TrimSuffix(full, []byte("\n")), []byte("\r"))
		blank := (lineBytes == 1 && bytes.Equal(part, []byte("\n"))) || (lineBytes == 2 && bytes.Equal(part, []byte("\r\n")))
		if blank {
			dispatch()
			event = nil
			size = 0
			overflow = false
		} else if !overflow && bytes.HasPrefix(full, []byte("data:")) {
			data := bytes.TrimPrefix(bytes.TrimPrefix(full, []byte("data:")), []byte(" "))
			if len(event) > 0 {
				event = append(event, '\n')
			}
			event = append(event, data...)
		}
		if err != nil {
			if err != io.EOF {
				unavailable()
			}
			return
		}
	}
}
