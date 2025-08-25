// EXPECT: EXIT 3
int main() { 
    int a = !0;     // a = 1
    int b = !5;     // b = 0  
    int c = !(!1);  // c = !0 = 1
    int d = !!7;    // d = !0 = 1
    return a + b + c + d;  // 1 + 0 + 1 + 1 = 3
}