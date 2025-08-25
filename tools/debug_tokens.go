package main
import (
  "fmt"
  "io/ioutil"
  "os"
  lx "github.com/tinyrange/cc/internal/lexer"
)
func main(){
  if len(os.Args)<2{ fmt.Println("usage: debug_tokens <file>"); os.Exit(2) }
  data, _ := ioutil.ReadFile(os.Args[1])
  l := lx.New(string(data))
  for {
    t := l.Next()
    fmt.Printf("%d %q at %d:%d\n", t.Type, t.Lex, t.Line, t.Col)
    if t.Type == lx.EOF { break }
  }
}
