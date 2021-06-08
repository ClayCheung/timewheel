package timewheel

import (
	"container/list"
	"time"
)

const (
	selfAlignmentInterval = time.Hour
)

// TimeWheel 时间轮，限定于一圈时间为1天的场景
type TimeWheel struct {
	interval            time.Duration // 指针每隔多久往前移动一格
	ticker              *time.Ticker
	selfAlignmentTicker *time.Ticker
	slots               []*list.List // 时间轮槽
	// key: 定时器唯一标识 value: 定时器所在的槽, 主要用于删除定时器, 不会出现并发读写，不加锁直接访问
	// timer 的倒排索引
	timer             map[interface{}]int
	currentPos        int              // 当前指针指向哪一个槽
	slotNum           int              // 槽数量
	callback          Callback         // 定时器回调函数
	addTaskChannel    chan Task        // 新增任务 channel
	removeTaskChannel chan interface{} // 删除任务 channel
	stopChannel       chan bool        // 停止时间轮 channel
}

// Callback 延时任务回调函数
// TODO 搞成多参数？
type Callback func(interface{})

// Task 定时任务（是一个任务，可以创建定时器）
type Task struct {
	date   int64 // 时间戳格式的时间
	circle int
	key    interface{} // 定时器唯一标识, 用于删除定时器
	data   interface{} // 回调函数参数
}

// New 创建时间轮
func New(interval time.Duration, callback Callback) *TimeWheel {
	if interval <= 0 || callback == nil {
		return nil
	}
	slotNum := int(24 * time.Hour / interval)
	tw := &TimeWheel{
		interval:          interval,
		slots:             make([]*list.List, slotNum),
		timer:             make(map[interface{}]int),
		currentPos:        0,
		callback:          callback,
		slotNum:           slotNum,
		addTaskChannel:    make(chan Task),
		removeTaskChannel: make(chan interface{}),
		stopChannel:       make(chan bool),
	}

	tw.initSlots()

	return tw
}

// 初始化槽，每个槽指向一个双向链表
func (tw *TimeWheel) initSlots() {
	for i := 0; i < tw.slotNum; i++ {
		tw.slots[i] = list.New()
	}
}

// Start 启动时间轮
func (tw *TimeWheel) Start() {
	tw.ticker = time.NewTicker(tw.interval)
	tw.selfAlignmentTicker = time.NewTicker(selfAlignmentInterval)
	go tw.start()
}

// Stop 停止时间轮
func (tw *TimeWheel) Stop() {
	tw.stopChannel <- true
}

// AddTimer 添加定时器 key为定时器唯一标识，支持设置过去时间的任务
func (tw *TimeWheel) AddTimer(timestamp int64, key interface{}, data interface{}) {
	tw.addTaskChannel <- Task{date: timestamp, key: key, data: data}
}

// RemoveTimer 删除定时器 key为添加定时器时传递的定时器唯一标识
func (tw *TimeWheel) RemoveTimer(key interface{}) {
	if key == nil {
		return
	}
	tw.removeTaskChannel <- key
}

func (tw *TimeWheel) start() {
	tw.selfAlignment()
	for {
		select {
		case <-tw.ticker.C:
			tw.tickHandler()
		case <-tw.selfAlignmentTicker.C:
			tw.selfAlignment()
		case task := <-tw.addTaskChannel:
			tw.addTask(&task)
		case key := <-tw.removeTaskChannel:
			tw.removeTask(key)
		case <-tw.stopChannel:
			tw.ticker.Stop()
			return
		}
	}
}

func (tw *TimeWheel) tickHandler() {
	l := tw.slots[tw.currentPos]
	tw.scanAndRunTask(l)
	if tw.currentPos == tw.slotNum-1 {
		tw.currentPos = 0
	} else {
		tw.currentPos++
	}
}

// 扫描链表中过期定时器, 并执行回调函数
func (tw *TimeWheel) scanAndRunTask(l *list.List) {
	for e := l.Front(); e != nil; {
		task := e.Value.(*Task)
		if task.circle > 0 {
			task.circle--
			e = e.Next()
			continue
		}
		// TODO needs goroutines pool
		go tw.callback(task.data)
		next := e.Next()
		l.Remove(e)
		if task.key != nil {
			delete(tw.timer, task.key)
		}
		e = next
	}
}

// 新增任务到链表中
func (tw *TimeWheel) addTask(task *Task) {
	pos, circle := tw.getPositionAndCircle(task.date)
	task.circle = circle
	tw.slots[pos].PushBack(task)
	if task.key != nil {
		tw.timer[task.key] = pos
	}
}

// 获取定时器在槽中的位置, 时间轮需要转动的圈数
func (tw *TimeWheel) getPositionAndCircle(date int64) (pos int, circle int) {
	tm := time.Unix(date, 0)
	circle = int(tm.Sub(time.Now()).Hours() / 24)
	// tmSecond 传入 date 的时间相对于当天 0 点的秒数
	tmSecond := 3600*tm.Hour() + 60*tm.Minute() + tm.Second()
	pos = tmSecond/int(tw.interval/time.Second) + 1
	return
}

// 从链表中删除任务
func (tw *TimeWheel) removeTask(key interface{}) {
	// 获取定时器所在的槽
	position, ok := tw.timer[key]
	if !ok {
		return
	}
	// 获取槽指向的链表
	l := tw.slots[position]
	for e := l.Front(); e != nil; {
		task := e.Value.(*Task)
		if task.key == key {
			delete(tw.timer, task.key)
			l.Remove(e)
		}
		e = e.Next()
	}
}

// 自动校准时间，调整 currentPos
func (tw *TimeWheel) selfAlignment() {
	now := time.Now()
	// 当前时间相对于当天 0 点的秒数
	nowSecond := 3600*now.Hour() + 60*now.Minute() + now.Second()
	pos := nowSecond/int(tw.interval/time.Second) + 1
	tw.alignment(pos)
}

// 直接外部调用 alignment 是不安全的，可能出现 race
func (tw *TimeWheel) alignment(pos int) {
	tw.currentPos = pos
}
