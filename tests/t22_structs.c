// EXPECT: EXIT 5
struct S { int x; int y; };
int main(){ struct S s; s.x=2; s.y=3; return s.x+s.y; }

