package normalizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/bblfsh/sdk.v1/uast"
	"gopkg.in/bblfsh/sdk.v1/uast/ann"
)

// the directories with the fixtures for the integration and the unit
// tests (we will be reusing some fixtures from the integration tests
// in this unit tests).
const (
	integration = "../../fixtures"
	unit        = "fixtures"
)

// containRoles checks if a node contains the roles in andRoles while not containing
// any of the roles in notRoles
func containsRoles(n *uast.Node, andRoles []uast.Role, notRoles []uast.Role) bool {
	roleMap := make(map[uast.Role]bool)
	for _, r := range n.Roles {
		roleMap[r] = true
	}
	for _, r := range andRoles {
		if !roleMap[r] {
			return false
		}
	}
	if notRoles != nil {
		for _, r := range notRoles {
			if roleMap[r] {
				return false
			}
		}
	}
	return true
}

// Reads a native AST encoded in JSON from a file in the fixture directory.
func getFixture(dir, file string) (data map[string]interface{}, err error) {
	path := filepath.Join(dir, file)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if errClose := f.Close(); err == nil {
			err = errClose
		}
	}()

	d := json.NewDecoder(f)
	if err := d.Decode(&data); err != nil {
		return nil, err
	}

	ast, ok := data["ast"]
	if !ok {
		return nil, fmt.Errorf("ast object not found")
	}

	asMap, ok := ast.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("cannot convert ast to map")
	}

	return asMap, nil
}

// Reads a native AST encoded in JSON from a file in the fixture directory, runs
// ToNode on it and annotates it with AnnotationsRules.
func annotateFixture(dir, file string) (*uast.Node, error) {
	f, err := getFixture(dir, file)
	if err != nil {
		return nil, err
	}

	n, err := ToNode.ToNode(f)
	if err != nil {
		return nil, err
	}

	err = AnnotationRules.Apply(n)
	if err != nil {
		return nil, err
	}

	return n, err
}

// find returns an slice with all the nodes in the tree that contains a set of roles
// while not container another set of them.
func find(tree *uast.Node, andRoles []uast.Role, notRoles []uast.Role) []*uast.Node {
	var found []*uast.Node
	_find(tree, andRoles, notRoles, &found)
	return found
}

func _find(n *uast.Node, andRoles []uast.Role, notRoles []uast.Role, ret *[]*uast.Node) {
	if containsRoles(n, andRoles, notRoles) {
		*ret = append(*ret, n)
	}
	for _, child := range n.Children {
		_find(child, andRoles, notRoles, ret)
	}
}

func mustBeTheSame(t *testing.T, expected, obtained []string) {
	sort.Strings(expected)
	sort.Strings(obtained)
	require.Equal(t, expected, obtained)
}

// returns a slice whith the tokens in the given nodes.
func tokens(s ...*uast.Node) []string {
	var ret []string
	for _, e := range s {
		ret = append(ret, e.Token)
	}
	return ret
}

func TestAnnotationsErrorIfRootIsNotFile(t *testing.T) {
	require := require.New(t)
	_, err := annotateFixture(unit, "root_is_not_file.json")
	require.Error(err)

	detailedError, ok := err.(ann.RuleError)
	require.True(ok)
	require.True(ErrRootMustBeFile.Is(detailedError.Inner()))
}

func TestAnnotationsRootIsFile(t *testing.T) {
	require := require.New(t)
	n, err := annotateFixture(integration, "var_declaration.bash.native")
	require.NoError(err)
	require.Contains(n.Roles, uast.File)
}

func TestAnnotationsCommentAreComments(t *testing.T) {
	n, err := annotateFixture(integration, "comments.bash.native")
	require.NoError(t, err)

	expected := []string{
		"# comment 1",
		"# comment 2",
	}
	obtained := tokens(find(n, []uast.Role{uast.Comment}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}
func TestAnnotationsShebangIsComment(t *testing.T) {
	n, err := annotateFixture(integration, "shebang.bash.native")
	require.NoError(t, err)

	expected := []string{"#!/bin/bash\n"}
	obtained := tokens(find(n, []uast.Role{uast.Comment}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}

func TestAnnotationsShebangIsDocumentation(t *testing.T) {
	n, err := annotateFixture(integration, "shebang.bash.native")
	require.NoError(t, err)

	expected := []string{"#!/bin/bash\n"}
	obtained := tokens(find(n, []uast.Role{uast.Documentation}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}

func TestAnnotationsOrdinatyCommentsAreNotDocumentation(t *testing.T) {
	n, err := annotateFixture(integration, "comments.bash.native")
	require.NoError(t, err)

	var expected []string // we don't expect to find any documentation in the file
	obtained := tokens(find(n, []uast.Role{uast.Documentation}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}

func TestAnnotationsVariableDeclaration(t *testing.T) {
	n, err := annotateFixture(integration, "var_declaration.bash.native")
	require.NoError(t, err)

	var expected = []string{"a"}
	obtained := tokens(find(n, []uast.Role{uast.Identifier}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}

func TestAnnotationsFunctionDeclaration(t *testing.T) {
	n, err := annotateFixture(integration, "function_declaration.bash.native")
	require.NoError(t, err)

	var expected = []string{"function"}
	obtained := tokens(find(n, []uast.Role{uast.Function, uast.Declaration}, []uast.Role{uast.Name, uast.Body})...)
	mustBeTheSame(t, expected, obtained)

	expected = []string{"foo"}
	obtained = tokens(find(n, []uast.Role{uast.Function, uast.Declaration, uast.Name}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)

	bodies := find(n, []uast.Role{uast.Body}, []uast.Role{})
	require.Equal(t, 1, len(bodies))

	blocks := find(n, []uast.Role{uast.Block}, []uast.Role{})
	require.Equal(t, 1, len(blocks))
}

func TestAnnotationsConditionals(t *testing.T) {
	n, err := annotateFixture(integration, "if.bash.native")
	require.NoError(t, err)

	// see readme.md, in the future we would like to check for IfBody,
	// IfConditional, but right now, this is the best we can do.
	var expected = []string{
		"if a1; then a2; fi",
		"if b1; then b2; else b3; fi",
		"if c1; then c2; elif c3; then c4; fi",
		"if d1; then d2; elif d3; then d4; else d5; fi",
		"if e1\nthen e2\nelif e3\nthen e4\nelse e5\nfi",
		"if [ -a file ]; then /bin/true; fi",
		"if [ \"a\" == \"b\" ]; then /bin/true; fi",
		"if [ \"c\" == \"d\" ]\n    then\n        /bin/true\nfi",
	}
	obtained := tokens(find(n, []uast.Role{uast.If}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
	obtained = tokens(find(n, []uast.Role{uast.Statement}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}

func TestAnnotationsForLoop(t *testing.T) {
	n, err := annotateFixture(integration, "for.bash.native")
	require.NoError(t, err)

	// same problem as TestAnnotationsConditionals.
	var expected = []string{
		"for a; do b; done",
		"for c in d; do e; done",
		"for f in g; do\n    h\ndone",
	}
	obtained := tokens(find(n, []uast.Role{uast.For}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}

func TestAnnotationsWhileLoop(t *testing.T) {
	n, err := annotateFixture(integration, "while.bash.native")
	require.NoError(t, err)

	// same problem as TestAnnotationsConditionals.
	var expected = []string{
		"while a; do b; c; done",
		"while d\ndo e\nf\ndone",
		"while g; do\n  h\n  i\ndone",
	}
	obtained := tokens(find(n, []uast.Role{uast.While}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}

func TestAnnotationsUntilLoop(t *testing.T) {
	n, err := annotateFixture(integration, "until.bash.native")
	require.NoError(t, err)

	// same problem as TestAnnotationsConditionals.  This is the same as
	// the while tests, but we should check for the inverted condition
	// here, when the sdk is allows for it.
	var expected = []string{
		"until a; do b; c; done",
		"until d\ndo e\nf\ndone",
		"until g; do\n  h\n  i\ndone",
	}
	obtained := tokens(find(n, []uast.Role{uast.While}, []uast.Role{})...)
	mustBeTheSame(t, expected, obtained)
}