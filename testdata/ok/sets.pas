program sets(output);
type
  color = (red, green, blue);
  hue = set of color;
  small = set of 1..10;
  bits = packed set of 0..7;
var
  h, g: hue;
  s: small;
  b: boolean;
begin
  h := [];
  h := [red, green];
  h := [red..blue];
  h := h + g * h;
  h := h - g;
  b := red in h + g;
  s := [1..10]
end.
