# timewheel

# Feature
- Set tasks to be triggered by set time
- TimeWheel one circle is one day
- Automatically align the time every 1 hour

# Usage

```go
package main

import (
	"fmt"
	"time"
	
	"github.com/ClayCheung/timewheel"
)

func main()  {
	// initial timeWheel, set time goes interval and callback function
	tw := timewheel.New(1 * time.Second, func(data interface{}) {
		fmt.Println(data)
	})

	// Start timeWheel
	tw.Start()

	// Add timer
	// set trigger time(timestamp format)
	// set timer ID
	// set callback function's parameter
	tw.AddTimer(1623147042, "rule-1-start", "data-01"})
	tw.AddTimer(1623147102, "rule-1-end", "data-02")
	
	// Remove timer by ID
	tw.RemoveTimer("rule-1-start")
    
	// Stop TimeWheel
	//tw.Stop()

	select{}
}
```
