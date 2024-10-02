package grep

type Result struct {
	PatternID uint
	Path      string
	Content   string
	Comment   string
	Pattern   string
}

type Grepper interface {
	Grep(fs interface{}, options ...GrepOption) ([]Result, error)
	Release()
}
