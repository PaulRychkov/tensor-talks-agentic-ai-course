package models

// InterviewProgram представляет программу интервью.
type InterviewProgram struct {
	Questions []QuestionItem `json:"questions"`
}

// QuestionItem представляет один пункт программы (вопрос + теория).
// Topic — нормализованный тег подтемы (например, "theory_rag"), используется
// в study-режиме для группировки вопросов по подтемам в плане.
type QuestionItem struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Theory   string `json:"theory"`
	Order    int    `json:"order"`
	Topic    string `json:"topic,omitempty"`
	// Study-mode hierarchy: subtopic → point → question.
	// For interview/training modes these remain empty.
	Subtopic        string `json:"subtopic,omitempty"`
	PointID         string `json:"point_id,omitempty"`
	PointTitle      string `json:"point_title,omitempty"`
	PointTheory     string `json:"point_theory,omitempty"`
	QuestionInPoint int    `json:"question_in_point,omitempty"`
}
