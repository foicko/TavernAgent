package application

// 服务生命周期与运行接线：关闭顺序（先排空 worker，再关后台队列）与
// 观测/协作者的注入都在这里，service.go 只留回合流水线本身。

// SetCompactor 注入后台上下文压缩器（M4b）。
func (s *TurnService) SetCompactor(c *CompactorService) {
	s.compactor = c
	if c != nil {
		c.SetMetrics(s.metrics)
	}
}

// SetMetrics 注入运行计数器（观测用，nil 表示不计数）。
// 后台队列共用同一个读数：任务被丢弃和回合失败一样需要被看见。
func (s *TurnService) SetMetrics(m *RuntimeMetrics) {
	s.metrics = m
	if s.compactor != nil {
		s.compactor.SetMetrics(m)
	}
	if s.cognitive != nil {
		s.cognitive.SetMetrics(m)
	}
}

// Metrics 返回运行计数器（供观测端点读取）。
func (s *TurnService) Close() {
	s.closeOnce.Do(func() {
		s.lifecycleMu.Lock()
		s.closing = true
		s.stopWork()
		s.lifecycleMu.Unlock()
		s.workers.Wait()
		if s.compactor != nil {
			s.compactor.Close()
		}
		if s.cognitive != nil {
			s.cognitive.Close()
		}
	})
}

func (s *TurnService) beginWork() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closing {
		return false
	}
	s.workers.Add(1)
	return true
}
