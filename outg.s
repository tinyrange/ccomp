.text
.globl main
main:
  push %rbp
  mov %rsp, %rbp
  sub $32, %rsp
  lea g(%rip), %rcx
  mov %rcx, %rcx
  mov (%rcx), %rdx
  mov %rdx, %rax
  add $32, %rsp
  pop %rbp
  ret
  mov $0, %eax
  add $32, %rsp
  pop %rbp
  ret
.data
.globl g
g:
  .quad 5
