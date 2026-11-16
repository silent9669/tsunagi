package ui

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// ReadSecret reads one line from stdin without echoing it on a terminal.
// A piped stdin is read as is, so `echo $KEY | tsunagi login x` works.
func (t *Term) ReadSecret(prompt string) (string, error) {
	t.Printf("%s", prompt)
	if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
		setEcho(false)
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			if _, ok := <-ch; ok {
				setEcho(true)
				t.Printf("\n")
				os.Exit(130)
			}
		}()
		defer func() {
			signal.Stop(ch)
			close(ch)
			setEcho(true)
			t.Printf("\n")
		}()
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", err
	}
	return line, nil
}

func setEcho(on bool) {
	arg := "-echo"
	if on {
		arg = "echo"
	}
	cmd := exec.Command("stty", arg)
	cmd.Stdin = os.Stdin
	cmd.Run()
}
