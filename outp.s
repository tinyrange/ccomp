.text
.globl main
main:
  push %rbp
  mov %rsp, %rbp
  sub $32, %rsp
  mov $5, %rcx
  mov $5, %rax
  mov %rax, -8(%rbp)
  lea -8(%rbp), %rdx
  mov %rdx, %rcx
  mov (%rcx), %rcx
  mov %rcx, %rax
  add $32, %rsp
  pop %rbp
  ret
  mov $0, %eax
  add $32, %rsp
  pop %rbp
  ret
