# GoDD
This is a golang wrapper of the [DD](https://github.com/ddxoft/master)  virtual mouse & keyboard driver.

Tested and working on Windows (10 &11).


## How to Use
Please refer to [DDxoft](https://github.com/ddxoft/master).

Include the DLL in the .zip file in to your project.

```go
package main

import (
	"github.com/rpdg/godd"
)

// download the free version from https://github.com/ddxoft/master
const DdDll = "./dd43390.dll"

func main(){
    dd, err := godd.GetDD(DdDll)
	if err != nil {
		panic("Device initialization error, " + err.Error())
		return
	}
    defer func() {
		dd.Close()
	}()
    
    // mouse control functions
    dd.MoveTo(0, 0)
    dd.MoveRelative(50, 150)
    dd.SetMouseButtonState(godd.BtnLeftDown)
    time.Sleep(time.Millisecond * 50)
    dd.SetMouseButtonState(godd.BtnLeftUp)
    dd.ScrollWheel(godd.WheelBackward)
        
    // keyboard control functions
    dd.SendKeyEvent(godd.VK_F1, godd.KeyDown)
    time.Sleep(time.Millisecond * 50)
    dd.SendKeyEvent(godd.VK_F1, godd.KeyUp)
    dd.InputString("some text")   
    
    // shortcut functions
    dd.Click() // performs a single left mouse click
    dd.PressKey(godd.VK_SPACE, time.Millisecond * 100) // holds a key for specified duration
    dd.PressHotkey(godd.VK_LWIN, godd.VK_M) // simulates keyboard shortcuts, e.g. WIN + M
}
```

## License
The GoDD package is open-sourced software licensed under the Apache V2 license.

## Feedback & Contribute
Notify me of any issues, bugs, or improvements. Thanks 👍