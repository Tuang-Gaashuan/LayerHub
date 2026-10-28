package app

import "database/sql"

// Expectations include executing/assigned jobs so a concurrent start or transfer
// invalidates the review. Reordering never transfers a file or starts a printer.
type QueuePosition struct {
	ID    int64  `json:"id"`
	Order int64  `json:"order"`
	State string `json:"state"`
}

type QueueMove struct {
	PrinterID int64           `json:"printerId"`
	JobID     int64           `json:"jobId"`
	Move      string          `json:"move"`
	Expected  []QueuePosition `json:"expected"`
}

func (s *Service) MoveQueueJob(u User, in QueueMove) error {
	if in.PrinterID <= 0 || in.JobID <= 0 || len(in.Expected) == 0 || len(in.Expected) > 1000 || (in.Move != "up" && in.Move != "down" && in.Move != "first") {
		return invalid("请选择有效设备、等待任务和调整方向")
	}
	return s.transaction(func(tx *sql.Tx) error {
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		var active []Job
		var waiting []Job
		index := -1
		for _, j := range jobs {
			if j.Printer != in.PrinterID || !activeStates[j.State] {
				continue
			}
			active = append(active, j)
			if j.State == "queued" {
				if j.ID == in.JobID {
					index = len(waiting)
				}
				waiting = append(waiting, j)
			}
		}
		if len(active) != len(in.Expected) {
			return conflict("队列已变化，请重新读取后再调整")
		}
		for i, j := range active {
			if in.Expected[i] != (QueuePosition{j.ID, j.Order, j.State}) {
				return conflict("队列已变化，请重新读取后再调整")
			}
		}
		if index < 0 {
			return conflict("只能调整这台设备尚未传输的等待任务")
		}
		if !u.Admin && waiting[index].UserID != u.ID {
			return forbidden("无权调整其他成员的任务顺序")
		}
		destination := index
		switch in.Move {
		case "up":
			destination--
		case "down":
			destination++
		case "first":
			destination = 0
		}
		if destination < 0 || destination >= len(waiting) || destination == index {
			return conflict("任务已在该方向的边界，无需调整")
		}
		lo, hi := min(index, destination), max(index, destination)
		if !u.Admin {
			for _, j := range waiting[lo : hi+1] {
				if j.UserID != u.ID {
					return forbidden("成员只能调整自己连续等待的任务，不能越过其他成员；请联系管理员")
				}
			}
		}
		// Reuse these tasks' existing order slots. Other printers, prepared files,
		// executing jobs, stock reservations and print options remain untouched.
		ordered := append([]Job(nil), waiting...)
		moved := ordered[index]
		if destination < index {
			copy(ordered[destination+1:index+1], ordered[destination:index])
		} else {
			copy(ordered[index:destination], ordered[index+1:destination+1])
		}
		ordered[destination] = moved
		for i := lo; i <= hi; i++ {
			if i > 0 && waiting[i].Order <= waiting[i-1].Order {
				return conflict("队列顺序记录异常，请联系管理员核对")
			}
			if _, err = tx.Exec("UPDATE jobs SET queue_order=? WHERE id=? AND state='queued'", waiting[i].Order, ordered[i].ID); err != nil {
				return err
			}
		}
		return nil
	})
}
