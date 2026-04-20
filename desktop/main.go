package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func main() {
	if err := runDesktop(); err != nil {
		windows.MessageBox(0,
			windows.StringToUTF16Ptr(fmt.Sprintf("启动失败: %v", err)),
			windows.StringToUTF16Ptr("GPT Proxy"),
			windows.MB_OK|windows.MB_ICONERROR)
		os.Exit(1)
	}
}
