.text
.globl main
main:
  push %rbp
  mov %rsp, %rbp
  sub $16, %rsp
  mov $0, %rcx
  mov %rcx, %rax
  add $16, %rsp
  pop %rbp
  ret
  mov $0, %eax
  add $16, %rsp
  pop %rbp
  ret
