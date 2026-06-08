type Claims = Record<string, unknown> | null | undefined;

type InternalAccessInput = {
  userId?: string | null;
  sessionClaims?: Claims;
  adminUserIDs?: string | null;
  clerkSecretKey?: string | null;
};

function listFromClaim(value: unknown) {
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === "string");
  }
  if (typeof value === "string") {
    return [value];
  }
  return [];
}

function hasClaimValue(claims: Claims, keys: string[], allowed: string[]) {
  if (!claims) return false;
  return keys.some((key) => listFromClaim(claims[key]).some((value) => allowed.includes(value)));
}

export function canUseInternalTools({
  userId,
  sessionClaims,
  adminUserIDs,
  clerkSecretKey,
}: InternalAccessInput) {
  if (!clerkSecretKey?.trim()) {
    return true;
  }

  const subject = userId?.trim();
  if (!subject) {
    return false;
  }

  const explicitAdmins = (adminUserIDs ?? "")
    .split(",")
    .map((id) => id.trim())
    .filter(Boolean);
  if (explicitAdmins.includes(subject)) {
    return true;
  }

  return (
    hasClaimValue(sessionClaims, ["org_role", "orgRole", "role"], ["org:admin", "admin"]) ||
    hasClaimValue(sessionClaims, ["org_permissions", "orgPermissions", "permissions"], ["org:admin"])
  );
}
