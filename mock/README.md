# Example
"Handle"
   | "WithQueryApiTimeout"
       return SomeWrap(ctx),...
      | "BaseTemplate.ValidationParams"

There exists some thing called context leaking.
i.e. a context is meant to expire after the function call ends, 
but it was expored by some deep callee to external caller, making the context leak.

This leads to incorrect span, and even causes dead lock with opentracing span.
```go
A(ctx):
   subCtx = WithSpan(ctx)
       B(ctx) (context.Context):
           return ctx
```

# Solution
```go
   dont put all data into one context:
   function scope context, can only live inside current function
   inter-function scope context, can exists event after the function ends.


   should override get value method, if it was marked end, then it shall be treated like deleted.
```


Because all values are stored inside context.valueCtx, so we can pop funcScopeCtx until current.

emptyCtx
 | x0 valueCtx
   | x1 valueCtx
     | x2 valueCtx {x1,span}
       | return x3 {x2, kv}
       

double ctx implementation:
   emptyCtx
   | valueCtx{begin: funcCtx}
     | ....  (normal with value uses this)

for setup and retrieve mock, get or create the only valueCtx that contains funcCtx(a pointer), update using funcCtx.ptr = funcCtx.ptr.WithValue(k,v)
never expose funcCtx to outside.

function body: 
   ptr := funcCtx.ptr
   funcCtx.ptr = funcCtx.ptr.WithValue(k,v)
   defer funcCtx.ptr=ptr
   pass ctx to processors
```go
This ctx is called package private context.


for packages not provided us but violatile the rule, we can find the point that we *immediately* calls into them, use a special.




type ctx struct {

}

func (c *ctx) Value(key interface{}) interface{} {

}

c = context.WithValue(c,k,v)
```

# Workaround
The simplest workaroud thing is to not start span with functions returns context, plus not function scope context to that. 

The 100% reliable thing is to not call it. 