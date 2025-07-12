package unit

import (
	"reflect"
	"testing"
	"time"

	"github.com/goef-team/goef/dialects"
	"github.com/goef-team/goef/metadata"
	"github.com/goef-team/goef/query"
)

// TestEntity for expression testing
type TestEntity struct {
	ID        int64     `goef:"primary_key,auto_increment"`
	Name      string    `goef:"required,max_length:100"`
	Email     string    `goef:"required,unique,max_length:255"`
	Age       int       `goef:"min:0,max:150"`
	IsActive  bool      `goef:"default:true"`
	CreatedAt time.Time `goef:"created_at"`
	UpdatedAt time.Time `goef:"updated_at"`
}

func (e *TestEntity) GetTableName() string {
	return dialects.TestEntitiesTable
}

func (e *TestEntity) GetID() interface{} {
	return e.ID
}

func (e *TestEntity) SetID(id interface{}) {
	if v, ok := id.(int64); ok {
		e.ID = v
	}
}
func TestExpressionTranslation(t *testing.T) {
	dialect := dialects.NewPostgreSQLDialect(nil)
	metadataRegistry := metadata.NewRegistry()
	translator := query.NewSQLTranslator(dialect, metadataRegistry)

	tests := []struct {
		name         string
		expression   query.Expression
		expectedSQL  string
		expectedArgs []interface{}
	}{
		{
			name: "Simple equality",
			expression: &query.BinaryExpression{
				Left:     &query.MemberExpression{Member: "Name"},
				Right:    &query.ConstantExpression{Value: "John"},
				Operator: query.Equal,
			},
			expectedSQL:  `"name" = $1`,
			expectedArgs: []interface{}{"John"},
		},
		{
			name: "Greater than comparison",
			expression: &query.BinaryExpression{
				Left:     &query.MemberExpression{Member: "Age"},
				Right:    &query.ConstantExpression{Value: 18},
				Operator: query.GreaterThan,
			},
			expectedSQL:  `"age" > $1`,
			expectedArgs: []interface{}{18},
		},
		{
			name: "String contains",
			expression: &query.MethodCallExpression{
				Object:    &query.MemberExpression{Member: "Name"},
				Method:    "contains",
				Arguments: []query.Expression{&query.ConstantExpression{Value: "John"}},
			},
			expectedSQL:  `"name" LIKE '%' || $1 || '%'`,
			expectedArgs: []interface{}{"John"},
		},
		{
			name: "Complex AND expression",
			expression: &query.BinaryExpression{
				Left: &query.BinaryExpression{
					Left:     &query.MemberExpression{Member: "Age"},
					Right:    &query.ConstantExpression{Value: 18},
					Operator: query.GreaterThan,
				},
				Right: &query.BinaryExpression{
					Left:     &query.MemberExpression{Member: "IsActive"},
					Right:    &query.ConstantExpression{Value: true},
					Operator: query.Equal,
				},
				Operator: query.And,
			},
			expectedSQL:  `"age" > $1 AND "is_active" = $2`,
			expectedArgs: []interface{}{18, true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql, args, err := translator.Translate(test.expression)
			if err != nil {
				t.Fatalf("Failed to translate expression: %v", err)
			}

			if sql != test.expectedSQL {
				t.Errorf("Expected SQL: %s, got: %s", test.expectedSQL, sql)
			}

			if len(args) != len(test.expectedArgs) {
				t.Errorf("Expected %d args, got %d", len(test.expectedArgs), len(args))
			}

			for i, expectedArg := range test.expectedArgs {
				if i < len(args) && args[i] != expectedArg {
					t.Errorf("Expected arg %d to be %v, got %v", i, expectedArg, args[i])
				}
			}
		})
	}
}

func TestExpressionString(t *testing.T) {
	expr := &query.BinaryExpression{
		Left:     &query.MemberExpression{Member: "Name"},
		Right:    &query.ConstantExpression{Value: "John"},
		Operator: query.Equal,
	}

	expected := "Binary(Member(.Name) = Constant(John))"
	if expr.String() != expected {
		t.Errorf("Expected: %s, got: %s", expected, expr.String())
	}
}

func TestBinaryOperatorString(t *testing.T) {
	tests := []struct {
		operator query.BinaryOperator
		expected string
	}{
		{query.Equal, "="},
		{query.NotEqual, "<>"},
		{query.LessThan, "<"},
		{query.GreaterThan, ">"},
		{query.And, "AND"},
		{query.Or, "OR"},
		{query.Like, "LIKE"},
		{query.In, "IN"},
	}
	for _, test := range tests {
		t.Run(test.operator.String(), func(t *testing.T) {
			if test.operator.String() != test.expected {
				t.Errorf("Expected: %s, got: %s", test.expected, test.operator.String())
			}
		})
	}
}

func TestUnaryOperatorString(t *testing.T) {
	tests := []struct {
		operator query.UnaryOperator
		expected string
	}{
		{query.Not, "NOT"},
		{query.Negate, "-"},
		{query.IsNull, "IS NULL"},
		{query.IsNotNull, "IS NOT NULL"},
	}

	for _, test := range tests {
		t.Run(test.operator.String(), func(t *testing.T) {
			if test.operator.String() != test.expected {
				t.Errorf("Expected: %s, got: %s", test.expected, test.operator.String())
			}
		})
	}
}

func TestLambdaExpression(t *testing.T) {
	lambda := &query.LambdaExpression{
		Parameters: []query.ParameterExpression{
			{Name: "u", ExprType: reflect.TypeOf(TestEntity{})},
		},
		Body: &query.BinaryExpression{
			Left:     &query.MemberExpression{Member: "Name"},
			Right:    &query.ConstantExpression{Value: "John"},
			Operator: query.Equal,
		},
	}

	expected := "Lambda((u) => Binary(Member(.Name) = Constant(John)))"
	if lambda.String() != expected {
		t.Errorf("Expected: %s, got: %s", expected, lambda.String())
	}
}

func TestExpressionBuilders(t *testing.T) {
	// Test Equal builder
	expr := query.EqualEx(func(u TestEntity) interface{} { return u.Name }, "John")
	if expr == nil {
		t.Error("Expected non-nil expression from Equal builder")
	}

	// Test NotEqual builder
	expr = query.NotEqualEx(func(u TestEntity) interface{} { return u.Age }, 0)
	if expr == nil {
		t.Error("Expected non-nil expression from NotEqual builder")
	}

	// Test GreaterThan builder
	expr = query.GreaterThanEx(func(u TestEntity) interface{} { return u.Age }, 18)
	if expr == nil {
		t.Error("Expected non-nil expression from GreaterThan builder")
	}

	// Test Contains builder
	expr = query.ContainsEx(func(u TestEntity) string { return u.Name }, "Jo")
	if expr == nil {
		t.Error("Expected non-nil expression from Contains builder")
	}
}
