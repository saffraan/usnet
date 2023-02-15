#include "hook.h"

extern int hook_begin(uintptr_t handle);
extern int hook_end(uintptr_t handle);
extern int go_fn_call(uintptr_t handle);

int loop_wrapper(void *arg){
    int ret = 0;
    loop_params* p = (loop_params*)(arg); 
    if (ret = hook_begin(p->begin) < 0) {
        return ret;
    }

    if (ret = go_fn_call(p->fn) < 0) {
        return ret;
    }

    return hook_end(p->end);
}

void ff_run_wrap( void *arg) {
    ff_run(loop_wrapper, arg);
    return;
}

void sys_run_wrap(void *arg){
    int ret = 0;
    while (ret >= 0)
    {
        /* code */
        ret = loop_wrapper(arg);
    }
    return;
}