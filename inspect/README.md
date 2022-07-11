# bug records
## 
## duplicate error
```
func (BaseTemplate) FinishProcess(ctx context.Context, req Request, resp Response, err error) error {
    ->
func (unused_Recv0 BaseTemplate) FinishProcess(ctx context.Context, req Request, resp Response, err error)( err error)
```

## name duplicate 
```
func (dao *batchPriceDiscountRuleDAOImpl) QueryRecordBySql(ctx context.Context, sql string, filter dao.IPriceDiscountRuleFilter)( Resp_0 []*model.PriceDiscountRule)
```

## go mod replace: wrong when replace target is relative.
should replace with absolute directory, unless starts with ./

## new line type parameters should be stripped, otherwise they cause unmap
```
func A(ctx context.Context,
T int,
)
```

## internal packages
```
could not import google.golang.org/grpc/internal/resolver (invalid use of internal package google.golang.org/grpc/internal/resolver)
```

# vet generated code
```bash
go vet ./test/mock_gen/...
```