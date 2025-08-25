// EXPECT: EXIT 3
int main() {
    int x = 42;
    int *p = &x;
    int *q = p + 3;  // pointer arithmetic: add 3*8=24 bytes
    return q - p;    // should return element difference: 3
}