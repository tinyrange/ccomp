.text
.globl main
main:
  push %rbp
  mov %rsp, %rbp
  sub $64, %rsp
  mov $2, %rax
  mov %rax, -8(%rbp)
  jmp sw.cmp.0
switch.end: 
case.0: 
  mov $5, %rax
  mov %rax, -32(%rbp)
  mov -32(%rbp), %rax
  add $64, %rsp
  pop %rbp
  ret
default: 
  mov $0, %rax
  mov %rax, -48(%rbp)
  mov -48(%rbp), %rax
  add $64, %rsp
  pop %rbp
  ret
sw.cmp.0: 
  mov $2, %rax
  mov %rax, -16(%rbp)
  mov -8(%rbp), %rax
  cmp $2, %rax
  sete %al
  movzx %al, %rax
  mov %rax, -24(%rbp)
  cmp $0, -24(%rbp)
  jne case.0
  jmp default
  mov $0, %eax
  add $64, %rsp
  pop %rbp
  ret
