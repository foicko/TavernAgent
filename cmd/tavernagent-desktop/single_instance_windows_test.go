//go:build windows

package main

import "testing"

// TestAcquireNamedMutexRejectsDuplicate 守的是启动顺序这条回归。
//
// 单实例判定必须能可靠识别"已经有一份在跑"——只有它排在 app.Bootstrap 之前并返回
// false，run() 才能在抢数据目录锁之前退出，把"启动报错"变成"唤起已有窗口"。
func TestAcquireNamedMutexRejectsDuplicate(t *testing.T) {
	// 用测试专属名字：产品互斥体名是会话级的，若开发机上正开着桌面端，
	// 用产品名会让本测试的结果取决于"你有没有开着这个应用"。
	const name = "tavernagent-desktop-test-single-instance-mutex"

	unique, _, err := acquireNamedMutex(name)
	if err != nil {
		t.Fatalf("首次获取互斥体失败: %v", err)
	}
	if !unique {
		t.Fatal("首次获取应当抢到唯一所有权")
	}

	unique, _, err = acquireNamedMutex(name)
	if err != nil {
		t.Fatalf("重复获取不应报错（这是正常情况，不是异常）: %v", err)
	}
	if unique {
		t.Fatal("互斥体已被本进程持有时，再次获取应当报告已有实例")
	}
}

// TestAcquireNamedMutexSeparateNamesAreIndependent 确认判据来自互斥体名本身，
// 而不是"同进程里第二次调用就返回 false"这种假通过。
func TestAcquireNamedMutexSeparateNamesAreIndependent(t *testing.T) {
	first, _, err := acquireNamedMutex("tavernagent-desktop-test-mutex-a")
	if err != nil || !first {
		t.Fatalf("名字 A 应当抢到唯一所有权: unique=%v err=%v", first, err)
	}
	second, _, err := acquireNamedMutex("tavernagent-desktop-test-mutex-b")
	if err != nil {
		t.Fatalf("名字 B 获取失败: %v", err)
	}
	if !second {
		t.Fatal("不同名字的互斥体互不影响，名字 B 也应当是唯一实例")
	}
}
