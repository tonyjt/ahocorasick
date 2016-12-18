package ahocorasick

const (
	EnumHitTypeNone = iota // none, match时不支持此选项

	EnumHitTypeWord //[]int 命中的单词index

	EnumHitTypeWordCount //map[string]int  命中单词的数量

	EnumHitTypeWordIndex //map[string][]int  命中单词的位置

	EnumHitTypeIndexWord //map[int]string  内容中命中次的index以及内容
)
