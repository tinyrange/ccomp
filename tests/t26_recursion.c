// EXPECT: EXIT 120
int f(int n){ if(n<=1) return 1; return f(n-1)*n; }
int main(){ return f(5); }
