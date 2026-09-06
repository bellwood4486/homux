// profiles.go は .homux.toml の profiles 配列だけを書き換える（ADR 0008）。
//
// 全体を Unmarshal → Marshal で書き戻すと、init が生成した雛形のコメントや
// ユーザーが書いた ignore の整形が失われる。これは INV-10（CLI が repository を
// 変更しても、結果は CLI なしで理解可能であること）に反する。そこで
// "profiles = [" の "[" から対応する "]" までのバイト範囲だけを差し替え、
// ファイルの他の部分には 1 バイトも触れない。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bellwood4486/homux/internal/selector"
)

// ReplaceProfiles は path の profiles 配列を profiles で置き換える。
//
// profiles 配列の**内部**に書かれたコメントは失われる（spec §15 の既知の制限）。
// profiles キーが見つからない場合はエラーを返し、ファイルには何も書かない。
func ReplaceProfiles(path string, profiles []string) error {
	for _, p := range profiles {
		if !selector.ValidProfileName(p) {
			return fmt.Errorf("invalid profile name %q: must match ^[a-z0-9][a-z0-9_-]*$", p)
		}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(b)

	start, end, err := findProfilesArray(content)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	updated := content[:start] + renderProfiles(profiles) + content[end:]
	return writeFileAtomic(path, []byte(updated))
}

// findProfilesArray は profiles 配列の "[" の位置と "]" の次の位置を返す。
//
// TOML の文字列とコメントを飛ばしながら走査するため、配列内のコメントに
// 含まれる "]" や、文字列リテラル中の "]" を閉じ括弧と誤認しない。
func findProfilesArray(content string) (start, end int, err error) {
	const key = "profiles"

	i := 0
	atLineStart := true
	for i < len(content) {
		switch {
		case content[i] == '#':
			i = skipComment(content, i)
			atLineStart = false
		case content[i] == '"' || content[i] == '\'':
			i = skipString(content, i)
			atLineStart = false
		case content[i] == '\n':
			i++
			atLineStart = true
		case content[i] == ' ' || content[i] == '\t' || content[i] == '\r':
			i++
		case atLineStart && strings.HasPrefix(content[i:], key):
			if bracket, ok := arrayStartAfterKey(content, i+len(key)); ok {
				closing, err := matchBracket(content, bracket)
				if err != nil {
					return 0, 0, err
				}
				return bracket, closing + 1, nil
			}
			i += len(key)
			atLineStart = false
		default:
			i++
			atLineStart = false
		}
	}
	return 0, 0, fmt.Errorf("no top-level %q key found", key)
}

// arrayStartAfterKey は key の直後から "= [" が続くかを見る。続くなら "[" の
// 位置を返す。profiles が配列でない、あるいは別のキーの一部だった場合は false。
func arrayStartAfterKey(content string, i int) (int, bool) {
	for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
		i++
	}
	if i >= len(content) || content[i] != '=' {
		return 0, false
	}
	i++
	for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
		i++
	}
	if i >= len(content) || content[i] != '[' {
		return 0, false
	}
	return i, true
}

// matchBracket は open 位置の "[" に対応する "]" の位置を返す。
func matchBracket(content string, open int) (int, error) {
	depth := 0
	for i := open; i < len(content); {
		switch content[i] {
		case '#':
			i = skipComment(content, i)
		case '"', '\'':
			i = skipString(content, i)
		case '[':
			depth++
			i++
		case ']':
			depth--
			if depth == 0 {
				return i, nil
			}
			i++
		default:
			i++
		}
	}
	return 0, fmt.Errorf("unterminated profiles array")
}

// skipComment は "#" から行末までを飛ばし、次に読むべき位置を返す。
func skipComment(content string, i int) int {
	for i < len(content) && content[i] != '\n' {
		i++
	}
	return i
}

// skipString は文字列リテラルを飛ばし、次に読むべき位置を返す。
// basic string（"）ではバックスラッシュによるエスケープを解釈する。
func skipString(content string, i int) int {
	quote := content[i]
	i++
	for i < len(content) {
		if quote == '"' && content[i] == '\\' {
			i += 2
			continue
		}
		if content[i] == quote {
			return i + 1
		}
		i++
	}
	return i
}

// renderProfiles は profiles 配列のリテラルを組み立てる。1 要素 1 行に
// 正規化される（ADR 0008 の帰結）。profile 名の文法（spec §5.4）は
// エスケープを要する文字を許さないため、そのまま引用符で囲めばよい。
func renderProfiles(profiles []string) string {
	if len(profiles) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[\n")
	for _, p := range profiles {
		fmt.Fprintf(&b, "  %q,\n", p)
	}
	b.WriteString("]")
	return b.String()
}

// writeFileAtomic は一時ファイル + rename で書き込む。既存ファイルの
// パーミッションを引き継ぐ（homux はパーミッションを管理しない。ADR 0006）。
func writeFileAtomic(path string, content []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // rename 成功後は存在しないので害はない

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
