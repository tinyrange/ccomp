// EXPECT: EXIT 0
// Test that pointer subtraction returns element count, not byte difference
int main() {
    char c = 65;
    char *p = &c;
    return p - p;  // should be 0 elements between same pointer
}