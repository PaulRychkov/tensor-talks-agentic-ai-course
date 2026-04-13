package models

// InterviewProgram представляет программу интервью.
type InterviewProgram struct {
	Questions []QuestionItem `json:"questions"`
}

// QuestionItem представляет один пункт программы (вопрос + теория).
type QuestionItem struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Theory   string `json:"theory"`
	Order    int    `json:"order"`
}
