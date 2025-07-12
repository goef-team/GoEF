// Package query provides expression tree functionality for LINQ-style queries
package query

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
)

// Expression represents a query expression that can be translated to SQL
type Expression interface {
	// Accept visitor pattern for expression tree traversal
	Accept(visitor ExpressionVisitor) (interface{}, error)
	// Type returns the Go type this expression evaluates to
	Type() reflect.Type
	// String returns a string representation of the expression
	String() string
}

// ExpressionVisitor defines the visitor interface for expression tree traversal
type ExpressionVisitor interface {
	VisitConstant(expr *ConstantExpression) (interface{}, error)
	VisitParameter(expr *ParameterExpression) (interface{}, error)
	VisitMember(expr *MemberExpression) (interface{}, error)
	VisitBinary(expr *BinaryExpression) (interface{}, error)
	VisitUnary(expr *UnaryExpression) (interface{}, error)
	VisitMethodCall(expr *MethodCallExpression) (interface{}, error)
	VisitLambda(expr *LambdaExpression) (interface{}, error)
}

// ConstantExpression represents a constant value in an expression
type ConstantExpression struct {
	Value    interface{}
	ExprType reflect.Type
}

func (e *ConstantExpression) Accept(visitor ExpressionVisitor) (interface{}, error) {
	return visitor.VisitConstant(e)
}

func (e *ConstantExpression) Type() reflect.Type {
	return e.ExprType
}

func (e *ConstantExpression) String() string {
	return fmt.Sprintf("Constant(%v)", e.Value)
}

// ParameterExpression represents a parameter in an expression
type ParameterExpression struct {
	Name     string
	ExprType reflect.Type
}

func (e *ParameterExpression) Accept(visitor ExpressionVisitor) (interface{}, error) {
	return visitor.VisitParameter(e)
}

func (e *ParameterExpression) Type() reflect.Type {
	return e.ExprType
}

func (e *ParameterExpression) String() string {
	return fmt.Sprintf("Parameter(%s)", e.Name)
}

// MemberExpression represents access to a member (field/property)
type MemberExpression struct {
	Expression Expression
	Member     string
	ExprType   reflect.Type
}

func (e *MemberExpression) Accept(visitor ExpressionVisitor) (interface{}, error) {
	return visitor.VisitMember(e)
}

func (e *MemberExpression) Type() reflect.Type {
	return e.ExprType
}

// FIX: Handle nil Expression field properly
func (e *MemberExpression) String() string {
	if e.Expression == nil {
		return fmt.Sprintf("Member(.%s)", e.Member)
	}
	return fmt.Sprintf("Member(%s.%s)", e.Expression.String(), e.Member)
}

// BinaryExpression represents a binary operation
type BinaryExpression struct {
	Left     Expression
	Right    Expression
	Operator BinaryOperator
	ExprType reflect.Type
}

func (e *BinaryExpression) Accept(visitor ExpressionVisitor) (interface{}, error) {
	return visitor.VisitBinary(e)
}

func (e *BinaryExpression) Type() reflect.Type {
	return e.ExprType
}

func (e *BinaryExpression) String() string {
	return fmt.Sprintf("Binary(%s %s %s)", e.Left.String(), e.Operator.String(), e.Right.String())
}

// UnaryExpression represents a unary operation
type UnaryExpression struct {
	Operand  Expression
	Operator UnaryOperator
	ExprType reflect.Type
}

func (e *UnaryExpression) Accept(visitor ExpressionVisitor) (interface{}, error) {
	return visitor.VisitUnary(e)
}

func (e *UnaryExpression) Type() reflect.Type {
	return e.ExprType
}

func (e *UnaryExpression) String() string {
	return fmt.Sprintf("Unary(%s %s)", e.Operator.String(), e.Operand.String())
}

// MethodCallExpression represents a method call
type MethodCallExpression struct {
	Object    Expression
	Method    string
	Arguments []Expression
	ExprType  reflect.Type
}

func (e *MethodCallExpression) Accept(visitor ExpressionVisitor) (interface{}, error) {
	return visitor.VisitMethodCall(e)
}

func (e *MethodCallExpression) Type() reflect.Type {
	return e.ExprType
}

func (e *MethodCallExpression) String() string {
	args := make([]string, len(e.Arguments))
	for i, arg := range e.Arguments {
		args[i] = arg.String()
	}
	return fmt.Sprintf("MethodCall(%s.%s(%s))", e.Object.String(), e.Method, strings.Join(args, ", "))
}

// LambdaExpression represents a lambda expression
type LambdaExpression struct {
	Parameters []ParameterExpression
	Body       Expression
	ExprType   reflect.Type
}

func (e *LambdaExpression) Accept(visitor ExpressionVisitor) (interface{}, error) {
	return visitor.VisitLambda(e)
}

func (e *LambdaExpression) Type() reflect.Type {
	return e.ExprType
}

func (e *LambdaExpression) String() string {
	params := make([]string, len(e.Parameters))
	for i, param := range e.Parameters {
		params[i] = param.Name
	}
	return fmt.Sprintf("Lambda((%s) => %s)", strings.Join(params, ", "), e.Body.String())
}

// BinaryOperator represents binary operators
type BinaryOperator int

const (
	Equal BinaryOperator = iota
	NotEqual
	LessThan
	LessThanOrEqual
	GreaterThan
	GreaterThanOrEqual
	And
	Or
	Add
	Subtract
	Multiply
	Divide
	Modulo
	Like
	In
	NotIn
)

func (op BinaryOperator) String() string {
	switch op {
	case Equal:
		return "="
	case NotEqual:
		return "<>"
	case LessThan:
		return "<"
	case LessThanOrEqual:
		return "<="
	case GreaterThan:
		return ">"
	case GreaterThanOrEqual:
		return ">="
	case And:
		return "AND"
	case Or:
		return "OR"
	case Add:
		return "+"
	case Subtract:
		return "-"
	case Multiply:
		return "*"
	case Divide:
		return "/"
	case Modulo:
		return "%"
	case Like:
		return "LIKE"
	case In:
		return "IN"
	case NotIn:
		return "NOT IN"
	default:
		return "UNKNOWN"
	}
}

// UnaryOperator represents unary operators
type UnaryOperator int

const (
	Not UnaryOperator = iota
	Negate
	IsNull
	IsNotNull
)

func (op UnaryOperator) String() string {
	switch op {
	case Not:
		return "NOT"
	case Negate:
		return "-"
	case IsNull:
		return "IS NULL"
	case IsNotNull:
		return "IS NOT NULL"
	default:
		return "UNKNOWN"
	}
}

// SQLTranslator translates expression trees to SQL
type SQLTranslator struct {
	dialect    dialects.Dialect
	metadata   *metadata.Registry
	parameters []interface{}
	paramIndex int
}

// NewSQLTranslator creates a new SQL translator
func NewSQLTranslator(dialect dialects.Dialect, metadata *metadata.Registry) *SQLTranslator {
	return &SQLTranslator{
		dialect:    dialect,
		metadata:   metadata,
		parameters: make([]interface{}, 0),
		paramIndex: 1,
	}
}

// Translate converts an expression tree to SQL
func (t *SQLTranslator) Translate(expr Expression) (string, []interface{}, error) {
	t.parameters = make([]interface{}, 0)
	t.paramIndex = 1

	sql, err := expr.Accept(t)
	if err != nil {
		return "", nil, err
	}

	return sql.(string), t.parameters, nil
}

// VisitConstant translates constant expressions
func (t *SQLTranslator) VisitConstant(expr *ConstantExpression) (interface{}, error) {
	placeholder := t.dialect.Placeholder(t.paramIndex)
	t.parameters = append(t.parameters, expr.Value)
	t.paramIndex++
	return placeholder, nil
}

// VisitParameter translates parameter expressions
func (t *SQLTranslator) VisitParameter(expr *ParameterExpression) (interface{}, error) {
	// For now, parameters are handled as column references
	return t.dialect.Quote(expr.Name), nil
}

// VisitMember translates member access expressions
func (t *SQLTranslator) VisitMember(expr *MemberExpression) (interface{}, error) {
	// Convert member access to column reference
	// This is simplified - in practice we'd need more sophisticated mapping
	columnName := toSnakeCase(expr.Member)
	return t.dialect.Quote(columnName), nil
}

// VisitBinary translates binary expressions
func (t *SQLTranslator) VisitBinary(expr *BinaryExpression) (interface{}, error) {
	left, err := expr.Left.Accept(t)
	if err != nil {
		return nil, err
	}

	right, err := expr.Right.Accept(t)
	if err != nil {
		return nil, err
	}

	switch expr.Operator {
	case In, NotIn:
		// Handle IN operator specially
		return fmt.Sprintf("%s %s (%s)", left, expr.Operator.String(), right), nil
	default:
		return fmt.Sprintf("%s %s %s", left, expr.Operator.String(), right), nil
	}
}

// VisitUnary translates unary expressions
func (t *SQLTranslator) VisitUnary(expr *UnaryExpression) (interface{}, error) {
	operand, err := expr.Operand.Accept(t)
	if err != nil {
		return nil, err
	}

	switch expr.Operator {
	case IsNull, IsNotNull:
		return fmt.Sprintf("%s %s", operand, expr.Operator.String()), nil
	default:
		return fmt.Sprintf("%s %s", expr.Operator.String(), operand), nil
	}
}

// VisitMethodCall translates method call expressions
func (t *SQLTranslator) VisitMethodCall(expr *MethodCallExpression) (interface{}, error) {
	switch strings.ToLower(expr.Method) {
	case "contains":
		if len(expr.Arguments) != 1 {
			return nil, fmt.Errorf("contains method requires exactly one argument")
		}

		obj, err := expr.Object.Accept(t)
		if err != nil {
			return nil, err
		}

		arg, err := expr.Arguments[0].Accept(t)
		if err != nil {
			return nil, err
		}

		return fmt.Sprintf("%s LIKE '%%' || %s || '%%'", obj, arg), nil

	case "startswith":
		if len(expr.Arguments) != 1 {
			return nil, fmt.Errorf("startsWith method requires exactly one argument")
		}

		obj, err := expr.Object.Accept(t)
		if err != nil {
			return nil, err
		}

		arg, err := expr.Arguments[0].Accept(t)
		if err != nil {
			return nil, err
		}

		return fmt.Sprintf("%s LIKE %s || '%%'", obj, arg), nil

	case "endswith":
		if len(expr.Arguments) != 1 {
			return nil, fmt.Errorf("endsWith method requires exactly one argument")
		}

		obj, err := expr.Object.Accept(t)
		if err != nil {
			return nil, err
		}

		arg, err := expr.Arguments[0].Accept(t)
		if err != nil {
			return nil, err
		}

		return fmt.Sprintf("%s LIKE '%%' || %s", obj, arg), nil

	default:
		return nil, fmt.Errorf("unsupported method: %s", expr.Method)
	}
}

// VisitLambda translates lambda expressions
func (t *SQLTranslator) VisitLambda(expr *LambdaExpression) (interface{}, error) {
	// For lambda expressions, we translate the body
	return expr.Body.Accept(t)
}

// toSnakeCase converts PascalCase to snake_case
func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// Expression builders for common scenarios
func EqualEx[T any](member func(T) interface{}, value interface{}) Expression {
	return &BinaryExpression{
		Left:     &MemberExpression{Member: extractMemberName(member)},
		Right:    &ConstantExpression{Value: value},
		Operator: Equal,
	}
}

func NotEqualEx[T any](member func(T) interface{}, value interface{}) Expression {
	return &BinaryExpression{
		Left:     &MemberExpression{Member: extractMemberName(member)},
		Right:    &ConstantExpression{Value: value},
		Operator: NotEqual,
	}
}

func GreaterThanEx[T any](member func(T) interface{}, value interface{}) Expression {
	return &BinaryExpression{
		Left:     &MemberExpression{Member: extractMemberName(member)},
		Right:    &ConstantExpression{Value: value},
		Operator: GreaterThan,
	}
}

func LessThanEx[T any](member func(T) interface{}, value interface{}) Expression {
	return &BinaryExpression{
		Left:     &MemberExpression{Member: extractMemberName(member)},
		Right:    &ConstantExpression{Value: value},
		Operator: LessThan,
	}
}

func ContainsEx[T any](member func(T) string, value string) Expression {
	return &MethodCallExpression{
		Object:    &MemberExpression{Member: extractMemberName(member)},
		Method:    "Contains",
		Arguments: []Expression{&ConstantExpression{Value: value}},
	}
}

// extractMemberName extracts the member name from a function using reflection
func extractMemberName(fn interface{}) string {
	// Get the function's runtime info
	fnValue := reflect.ValueOf(fn)
	if fnValue.Kind() != reflect.Func {
		return "unknown"
	}

	// For simplicity, we'll extract the name from the function pointer
	// This is a basic implementation - a full implementation would need
	// sophisticated AST analysis or compile-time code generation
	pc := runtime.FuncForPC(fnValue.Pointer())
	if pc != nil {
		name := pc.Name()
		// Extract the last part after the dot
		if lastDot := strings.LastIndex(name, "."); lastDot != -1 {
			name = name[lastDot+1:]
		}
		// Remove function signature suffixes
		if hyphen := strings.Index(name, "-"); hyphen != -1 {
			name = name[:hyphen]
		}
		return name
	}

	return "member"
}
