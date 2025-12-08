export function displayProvisionTypes(provisionType?: string) {
  if (!provisionType) {
    return "Unknown";
  }
  switch (provisionType) {
    case "external":
      return "Externally";
    case "internal":
      return "Platform";
  }
}
