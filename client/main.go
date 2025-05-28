package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func main() {
	app := tview.NewApplication()

	// Khởi động với form login
	if err := app.SetRoot(buildLoginForm(app), true).Run(); err != nil {
		panic(err)
	}
}

// Tạo lại form đăng nhập mới mỗi lần cần hiển thị lại
func buildLoginForm(app *tview.Application) *tview.Form {
	form := tview.NewForm()

	form.AddInputField("Username", "", 20, nil, nil)
	form.AddPasswordField("Password", "", 20, '*', nil)
	form.AddButton("Login", func() {
		username := form.GetFormItemByLabel("Username").(*tview.InputField).GetText()
		password := form.GetFormItemByLabel("Password").(*tview.InputField).GetText()

		conn, err := net.Dial("tcp", "localhost:8080")
		if err != nil {
			showModal(app, fmt.Sprintf("Lỗi kết nối: %v", err), func() {
				app.Stop()
			})
			return
		}

		// Đọc chào mừng
		buf := make([]byte, 1024)
		_, err = conn.Read(buf)
		if err != nil {
			showModal(app, fmt.Sprintf("Lỗi đọc từ server: %v", err), func() {
				app.Stop()
			})
			return
		}

		// Gửi thông tin đăng nhập
		loginStr := fmt.Sprintf("%s|%s\n", username, password)
		_, err = conn.Write([]byte(loginStr))
		if err != nil {
			showModal(app, fmt.Sprintf("Lỗi gửi login: %v", err), func() {
				app.Stop()
			})
			return
		}

		// Đọc phản hồi đăng nhập
		n, err := conn.Read(buf)
		if err != nil {
			showModal(app, fmt.Sprintf("Lỗi phản hồi: %v", err), func() {
				app.Stop()
			})
			return
		}

		response := strings.TrimSpace(string(buf[:n]))
		if strings.Contains(response, "thành công") {
			showGameUI(app, conn, username)
		} else {
			showModal(app, "Đăng nhập thất bại: "+response, func() {
				newForm := buildLoginForm(app)
				app.SetRoot(newForm, true).SetFocus(newForm)
			})
		}
	})

	form.AddButton("Quit", func() {
		app.Stop()
	})

	form.SetBorder(true).SetTitle("TCR Login").SetTitleAlign(tview.AlignLeft)
	return form
}

// Hiển thị popup thông báo
func showModal(app *tview.Application, message string, doneFunc func()) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			doneFunc()
		})
	app.SetRoot(modal, false)
}

// Hiển thị giao diện chơi game
func showGameUI(app *tview.Application, conn net.Conn, username string) {
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() {
			app.Draw()
		})

	inputField := tview.NewInputField().
		SetLabel("Lệnh (attack/defend/skill/end): ").
		SetFieldWidth(30)

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(textView, 0, 1, false).
		AddItem(inputField, 3, 0, true)

	// Nhận dữ liệu từ server
	go func() {
		reader := bufio.NewReader(conn)
		for {
			msg, err := reader.ReadString('\n')
			if err != nil {
				app.QueueUpdateDraw(func() {
					textView.Write([]byte(fmt.Sprintf("[red]Mất kết nối: %v\n", err)))
				})
				break
			}
			app.QueueUpdateDraw(func() {
				textView.Write([]byte(fmt.Sprintf("[green]%s\n", strings.TrimSpace(msg))))
			})
		}
	}()

	// Gửi lệnh người chơi
	inputField.SetDoneFunc(func(key tcell.Key) {
		cmd := inputField.GetText()
		if cmd != "" {
			fmt.Fprintf(conn, "%s\n", cmd)
			inputField.SetText("")
		}
	})

	app.SetRoot(layout, true).SetFocus(inputField)
}
