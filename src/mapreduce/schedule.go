package mapreduce

import (
	"fmt"
)

// schedule starts and waits for all tasks in the given phase (Map or Reduce).
func (mr *Master) schedule(phase jobPhase) {
	var ntasks int
	var nios int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mr.files)
		nios = mr.nReduce
	case reducePhase:
		ntasks = mr.nReduce
		nios = len(mr.files)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, nios)

	// All ntasks tasks have to be scheduled on workers, and only once all of
	// them have been completed successfully should the function return.
	// Remember that workers may fail, and that any given worker may finish
	// multiple tasks.
	//
	// TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO
	//
	/*
	   调用两次一次map,一次reduce
	   可能任务比worker的数量多，所以需要给worker一个任务队列，每次一个，所有任务都成功才能返回
	   通过读取registerChan 获取worker信息，里面包含worker信息，如rpc addr.
	   有一些worker也许在schedule执行前就已经在registerChan中，有些则在执行过程中新添加
	   schedule需要利用所有的worker,包括启动后添加的
	*/
	/*
		Master参数分析：
		workers:注册到master中所有的worker
		registerChannel:注册到服务器的所有worker,与worker不同在于worker注册完成后就不会变化，registerChannel在线程中异步添加，可以重复添加
		所以，取可用worker直接从registerChannel获取即可，不需要读取workers
	*/
	/*
		实现分析：
			1.使用RPC调用worker的函数，可以仿注册函数
			2.异步执行所有的worker，所以每一个RPC的调用需要启线程
			3.任务可能比worker多，所以worker执行完成后需要让其他任务继续使用
			3.中间执行过程中若有失败情况，则需要重新执行此项任务
			4.实验手册中提示：Hint: You may find sync.WaitGroup useful.
	*/
	idleworker := make(chan string)
	jobs := make(chan int)
	doneworks := make(chan bool)
	go func() {
		for {
			newRegister := <-mr.registerChannel
			idleworker <- newRegister
		}
	}()

	go func() {
		for _, w := range mr.workers {
			idleworker <- w
		}
	}()

	go func() {
		for i := 0; i < ntasks; i++ {
			jobs <- i
		}
	}()

	go func() {
		for idx := range jobs {
			availableworker := <-idleworker
			go func(idx int, availableworker string) {
				debug("worker: %s", availableworker)
				args := new(DoTaskArgs)
				args.JobName = mr.jobName
				args.TaskNumber = idx
				args.NumOtherPhase = nios
				args.Phase = phase
				if phase == mapPhase {
					args.File = mr.files[idx]
				}
				ok := call(availableworker, "Worker.DoTask", args, new(struct{}))
				if ok == true {
					doneworks <- true
					idleworker <- availableworker
				} else {
					jobs <- idx
				}
			}(idx, availableworker)
		}
	}()

	for i := 0; i < ntasks; i++ {
		<-doneworks
	}
	close(jobs)

	// //ok := call(availableworker, "Worker.DoTask", args, new(struct{}))
	// var wg sync.WaitGroup

	// for i := 0; i < ntasks; i++ {
	// 	go func(i int) {
	// 		wg.Add(1)
	// 		args := new(DoTaskArgs)
	// 		if phase == "mapPhase" {
	// 			args.File = mr.files[i]
	// 		}
	// 		args.JobName = mr.jobName
	// 		args.NumOtherPhase = nios
	// 		args.Phase = phase
	// 		args.TaskNumber = i
	// 		for {
	// 			idleworker := <-mr.registerChannel
	// 			ok := call(idleworker, "Worker.DoTask", args, new(struct{}))
	// 			if ok {
	// 				wg.Done()
	// 				///放在if里面因为可能是worker本身错误，若是这样放在外面可能还会导致失败(失败继续就是了？？测试看看吧)
	// 				mr.registerChannel <- idleworker
	// 				break
	// 			}

	// 		}

	// 	}(i)
	// }

	// wg.Wait()

	fmt.Printf("Schedule: %v phase done\n", phase)
}
