.text
.globl main
main:
  push %rbp
  mov %rsp, %rbp
  sub $32, %rsp
  mov $-3, %rcx
  mov %rcx, %rax
  add $32, %rsp
  pop %rbp
  ret
  mov $0, %eax
  add $32, %rsp
  pop %rbp
  ret
