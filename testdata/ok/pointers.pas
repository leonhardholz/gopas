program pointers(output);
type
  node = record
    value: integer;
    next: ^node
  end;
  link = ^node;
var
  p: link;
begin
  p := nil;
  new(p);
  p^.value := 1;
  p^.next := nil
end.
