package services

import (
	"database/sql"
	"log"
	"time"
)

// StartFreeIPScanLoop 空闲 IP 定时扫描后台循环。
// 进程启动即运行（配合 crontab/systemd 保活，等于开机自启）：
// 首扫在启动 60 秒后落库一次，让页面立刻有数据；
// 之后每轮从 app_settings 读取间隔（分钟），改动无需重启即生效。
func StartFreeIPScanLoop(db *sql.DB) {
	go func() {
		// 启动后稍等片刻做首次扫描，避开进程启动高峰
		time.Sleep(60 * time.Second)
		runScheduledScan(db)

		for {
			interval := GetScanInterval(db)
			time.Sleep(time.Duration(interval) * time.Minute)
			runScheduledScan(db)
		}
	}()
}

// runScheduledScan 执行一次定时扫描并落库；失败只记日志不影响循环
func runScheduledScan(db *sql.DB) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[FreeIP] 定时扫描异常恢复: %v", r)
		}
	}()

	scan, err := GetFreeIPs(db, nil, true) // force：绕过内存缓存拿新数据
	if err != nil {
		log.Printf("[FreeIP] 定时扫描失败: %v", err)
		return
	}
	scanID, err := SaveScanToDB(db, scan, "auto")
	if err != nil {
		log.Printf("[FreeIP] 扫描结果落库失败: %v", err)
		return
	}
	log.Printf("[FreeIP] 定时扫描完成: 网段%d个 在线空闲%d个 (记录id=%d, 耗时%dms)",
		len(scan.Subnets), len(scan.Free), scanID, scan.DurationMs)
}
