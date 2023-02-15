#ifndef __HOOK_H__
#define __HOOK_H__

#include <stdint.h>
#include <ff_api.h>

typedef struct loop_params{
   uintptr_t begin;
   uintptr_t end;
   uintptr_t fn;
} loop_params;

int loop_wrapper(void *arg);
void ff_run_wrap(void *arg);
void sys_run_wrap(void *arg);
#endif