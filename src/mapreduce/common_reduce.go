package mapreduce

import (
	"encoding/json"
	"os"
)

// doReduce does the job of a reduce worker: it reads the intermediate
// key/value pairs (produced by the map phase) for this task, sorts the
// intermediate key/value pairs by key, calls the user-defined reduce function
// (reduceF) for each key, and writes the output to disk.
func doReduce(
	jobName string, // the name of the whole MapReduce job
	reduceTaskNumber int, // which reduce task this is
	nMap int, // the number of map tasks that were run ("M" in the paper)
	reduceF func(key string, values []string) string,
) {
	// TODO:
	// You will need to write this function.
	// You can find the intermediate file for this reduce task from map task number
	// m using reduceName(jobName, m, reduceTaskNumber).
	// Remember that you've encoded the values in the intermediate files, so you
	// will need to decode them. If you chose to use JSON, you can read out
	// multiple decoded values by creating a decoder, and then repeatedly calling
	// .Decode() on it until Decode() returns an error.
	//
	// You should write the reduced output in as JSON encoded KeyValue
	// objects to a file named mergeName(jobName, reduceTaskNumber). We require
	// you to use JSON here because that is what the merger than combines the
	// output from all the reduce tasks expects. There is nothing "special" about
	// JSON -- it is just the marshalling format we chose to use. It will look
	// something like this:
	//
	// enc := json.NewEncoder(mergeFile)
	// for key in ... {
	// 	enc.Encode(KeyValue{key, reduceF(...)})
	// }
	// file.Close()

	//buf, _ := ioutil.ReadFile(inFile)
	//kvs := mapF(inFile, string(buf))
	/*
		读取nmap个map worker上对应reducenum的文件（处理nmap个文件）。
		将结果合并产生一个结果文件。此结果文件是整个结果集合的一部分。（共有reducenum个文件分别在reduce worker上）
		处理方式：将nmap个文件map结果文件中相同中间key（中间指产生的中间集合）合并统计在一起，并将统计合计交由reduceF进行处理产生结果，并将总的处理结果存储在结果文件中
		结果文件由mergeName函数确定
	*/
	files := make([]*os.File, nMap)
	dec := make([]*json.Decoder, nMap)
	m := make(map[string][]string)
	for i := 0; i < nMap; i++ {
		name := reduceName(jobName, i, reduceTaskNumber)
		files[i], _ = os.Open(name)
		dec[i] = json.NewDecoder(files[i])
		for {
			var kv KeyValue
			err := dec[i].Decode(&kv)
			if err != nil {
				break
			} else {
				m[kv.Key] = append(m[kv.Key], kv.Value) // 将所有同样key存储在一起，之后交由reduceF进行处理
			}
		}

	}

	for i := 0; i < nMap; i++ {
		files[i].Close()
	}

	name := mergeName(jobName, reduceTaskNumber)

	file, _ := os.Create(name)
	enc := json.NewEncoder(file)
	for index, value := range m {
		enc.Encode(KeyValue{index, reduceF(index, value)})
	}

	file.Close()

}
