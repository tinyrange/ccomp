// EXPECT: EXIT 2
// Test that pointer subtraction returns element count rather than byte difference
int main() {
    int x = 42;
    int y = 99; 
    int *p = &x;
    int *q = &y;
    // Simulate q being 2 elements ahead of p by manual pointer arithmetic
    int *r = p + 2;  // r should be 16 bytes ahead of p (2 * 8 bytes)
    return r - p;    // should return 2 elements, not 16 bytes
}