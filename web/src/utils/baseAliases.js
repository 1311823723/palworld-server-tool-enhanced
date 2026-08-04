export const baseDisplayName = (base, index = 0) =>
  String(base?.custom_name || base?.display_name || "").trim() || `据点 ${index + 1}`;

export const validateBaseAliasName = (value, currentBaseID, bases = []) => {
  const name = String(value || "").trim();
  if (!name) return "请输入据点名称";
  if ([...name].length > 40) return "名称不能超过 40 个字符";
  if (/[\r\n\u0000-\u001f\u007f\u2028\u2029]/u.test(name)) return "名称不能包含换行或控制字符";
  const normalized = name.toLocaleLowerCase("zh-CN");
  const duplicate = bases.some((base, index) =>
    base?.id !== currentBaseID
    && base?.base_id !== currentBaseID
    && String(base?.custom_name || "").trim().toLocaleLowerCase("zh-CN") === normalized);
  return duplicate ? "当前存档中已有同名据点" : "";
};
