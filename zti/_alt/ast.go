package main

type Node interface {
	TokenLiteral() string
}

type Statement interface {
	Node
	statementNode()
}

type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

// ReadStmt: ERFASSE <Ident> ALS <Type> VON <Literal>
type ReadStmt struct {
	Token Token // ERFASSE
	Ident string
	Type  string
	From  string
}

func (rs *ReadStmt) statementNode()       {}
func (rs *ReadStmt) TokenLiteral() string { return rs.Token.Literal }

// TransformStmt: FORME VON <Src> NACH <Dst> MIT <Op>
type TransformStmt struct {
	Token Token // FORME
	Src   string
	Dst   string
	Op    string
}

func (ts *TransformStmt) statementNode()       {}
func (ts *TransformStmt) TokenLiteral() string { return ts.Token.Literal }

// CheckStmt: PRÜFE <Ident> WENN <Predicate> <Value>
type CheckStmt struct {
	Token     Token // PRÜFE
	Ident     string
	Predicate string
	Value     string
}

func (cs *CheckStmt) statementNode()       {}
func (cs *CheckStmt) TokenLiteral() string { return cs.Token.Literal }

// CommitStmt: BIND <Ident>
type CommitStmt struct {
	Token Token // BIND
	Ident string
}

func (cs *CommitStmt) statementNode()       {}
func (cs *CommitStmt) TokenLiteral() string { return cs.Token.Literal }

// HaltStmt: STOP <Code>
type HaltStmt struct {
	Token Token // STOP
	Code  int
}

func (hs *HaltStmt) statementNode()       {}
func (hs *HaltStmt) TokenLiteral() string { return hs.Token.Literal }
