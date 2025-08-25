// EXPECT: COMPILE-FAIL
int main(){ int i=0; while(1){ if(i==3) break; i=i+1; } return i; }

