.text
.globl _start
_start:
  call main
  mov %rax, %rdi   # exit code = main return
  mov $60, %rax    # sys_exit
  syscall

