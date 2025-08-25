.text
.globl main
main:
  push %rbp
  mov %rsp, %rbp
  sub $256, %rsp
  mov $0, %rcx
  lea -8(%rbp), %rdx
  mov $0, %r8
  mov %rdx, %r9
  add $0, %r9
  mov $2, %rdx
  mov %r9, %rcx
  mov $2, %rax
  mov %rax, (%rcx)
  lea -8(%rbp), %rdx
  mov $8, %r8
  mov %rdx, %r9
  add $8, %r9
  mov $3, %rdx
  mov %r9, %rcx
  mov $3, %rax
  mov %rax, (%rcx)
  lea -8(%rbp), %rdx
  mov $0, %r8
  mov %rdx, %r9
  add $0, %r9
  mov %r9, %rcx
  mov (%rcx), %rdx
  lea -8(%rbp), %r8
  mov $8, %rcx
  mov %r8, %r9
  add $8, %r9
  mov %r9, %rcx
  mov (%rcx), %rcx
  mov %rdx, %r8
  imul %rcx, %r8
  mov %r8, %rax
  add $256, %rsp
  pop %rbp
  ret
  mov $0, %eax
  add $256, %rsp
  pop %rbp
  ret
