import { readFile, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const CREDS_PATH = join(homedir(), ".gemini", "oauth_creds.json");
const CODE_ASSIST_ENDPOINT = "https://cloudcode-pa.googleapis.com";
const MODEL = process.env.GEMINI_MODEL ?? "gemini-2.0-flash";
const PROJECT_ID = process.env.GEMINI_PROJECT_ID ?? "airy-web-442906-m9";
const CLIENT_ID = process.env.GEMINI_OAUTH_CLIENT_ID ?? "";
const CLIENT_SECRET = process.env.GEMINI_OAUTH_CLIENT_SECRET ?? "";
const CODE_ASSIST_HEADERS = {
  "User-Agent": "google-api-nodejs-client/9.15.1",
  "X-Goog-Api-Client": "gl-node/22.17.0",
  "Client-Metadata": "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI",
};

async function loadCreds() {
  const raw = await readFile(CREDS_PATH, "utf8");
  return JSON.parse(raw);
}

async function refreshToken(refreshToken) {
  const body = new URLSearchParams({
    grant_type: "refresh_token",
    refresh_token: refreshToken,
    client_id: CLIENT_ID,
    client_secret: CLIENT_SECRET,
  });
  const resp = await fetch("https://oauth2.googleapis.com/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
  if (!resp.ok) {
    throw new Error(`Refresh failed: ${resp.status} ${await resp.text()}`);
  }
  return resp.json();
}

async function maybeStore(creds) {
  await writeFile(CREDS_PATH, JSON.stringify(creds, null, 2) + "\n", "utf8");
}

async function ensureAccessToken(creds) {
  const now = Date.now();
  const expires = typeof creds.expiry_date === "number" ? creds.expiry_date : 0;
  if (!creds.access_token || expires <= now - 60_000) {
    console.log("[gemini] Access token missing/expired. Refreshing…");
    const refreshed = await refreshToken(creds.refresh_token);
    creds.access_token = refreshed.access_token;
    if (typeof refreshed.expires_in === "number") {
      creds.expiry_date = now + refreshed.expires_in * 1000;
    }
    if (refreshed.refresh_token) {
      creds.refresh_token = refreshed.refresh_token;
    }
    await maybeStore(creds);
  }
  return creds.access_token;
}

function wrapRequest(prompt, projectId) {
  const request = {
    project: projectId || undefined,
    model: MODEL,
    request: {
      contents: [
        {
          role: "user",
          parts: [{ text: prompt }],
        },
      ],
    },
  };
  if (!request.project) {
    delete request.project;
  }
  return request;
}

function buildMetadata(projectId) {
  const metadata = {
    ideType: "IDE_UNSPECIFIED",
    platform: "PLATFORM_UNSPECIFIED",
    pluginType: "GEMINI",
  };
  if (projectId) {
    metadata.duetProject = projectId;
  }
  return metadata;
}

async function loadManagedProject(accessToken, projectId) {
  const body = {
    metadata: buildMetadata(projectId),
  };
  if (projectId) {
    body.cloudaicompanionProject = projectId;
  }

  const resp = await fetch(`${CODE_ASSIST_ENDPOINT}/v1internal:loadCodeAssist`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${accessToken}`,
      "Content-Type": "application/json",
      ...CODE_ASSIST_HEADERS,
    },
    body: JSON.stringify(body),
  });

  if (!resp.ok) {
    const text = await resp.text();
    console.warn(`[gemini] loadCodeAssist failed (${resp.status}): ${text}`);
    return null;
  }

  return resp.json();
}

function getDefaultTierId(allowedTiers) {
  if (!Array.isArray(allowedTiers) || allowedTiers.length === 0) {
    return undefined;
  }
  for (const tier of allowedTiers) {
    if (tier?.isDefault && tier?.id) {
      return tier.id;
    }
  }
  return allowedTiers[0]?.id;
}

async function wait(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function onboardManagedProject(accessToken, tierId, projectId, attempts = 10, delayMs = 5000) {
  const body = {
    tierId,
    metadata: buildMetadata(projectId),
  };
  if (tierId !== "FREE") {
    if (!projectId) {
      throw new Error("Non-free tier requires an explicit GEMINI_PROJECT_ID");
    }
    body.cloudaicompanionProject = projectId;
  }

  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const resp = await fetch(`${CODE_ASSIST_ENDPOINT}/v1internal:onboardUser`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
        ...CODE_ASSIST_HEADERS,
      },
      body: JSON.stringify(body),
    });

    if (!resp.ok) {
      const text = await resp.text();
      console.warn(`[gemini] onboardUser failed (${resp.status}): ${text}`);
      return undefined;
    }

    const payload = await resp.json();
    const managed = payload.response?.cloudaicompanionProject?.id;
    if (payload.done && managed) {
      return managed;
    }
    if (payload.done && projectId) {
      return projectId;
    }
    await wait(delayMs);
  }

  return undefined;
}

async function ensureProjectBinding(creds, accessToken) {
  if (PROJECT_ID) {
    return PROJECT_ID;
  }

  if (typeof creds.project_id === "string" && creds.project_id.trim()) {
    return creds.project_id.trim();
  }

  if (typeof creds.managed_project_id === "string" && creds.managed_project_id.trim()) {
    return creds.managed_project_id.trim();
  }

  console.log("[gemini] Resolving managed project…");
  const loadPayload = await loadManagedProject(accessToken, creds.project_id);
  if (loadPayload?.cloudaicompanionProject?.id) {
    const managed = loadPayload.cloudaicompanionProject.id;
    creds.managed_project_id = managed;
    await maybeStore(creds);
    return managed;
  }

  if (!loadPayload) {
    throw new Error(
      "loadCodeAssist failed and no project ID is available. Set GEMINI_PROJECT_ID or add project_id to oauth_creds.json.",
    );
  }

  const currentTierId = loadPayload.currentTier?.id;
  if (currentTierId && currentTierId !== "FREE" && !creds.project_id) {
    throw new Error(
      `Account is on tier ${currentTierId} but no project is set. Export GEMINI_PROJECT_ID before running.`,
    );
  }

  const tierId = getDefaultTierId(loadPayload.allowedTiers) ?? "FREE";
  if (tierId !== "FREE" && !creds.project_id) {
    throw new Error("Only the FREE tier can auto-onboard. Set GEMINI_PROJECT_ID.");
  }

  const managedProjectId = await onboardManagedProject(accessToken, tierId, creds.project_id);
  if (!managedProjectId) {
    throw new Error("Failed to onboard a managed project. Set GEMINI_PROJECT_ID and rerun.");
  }

  creds.managed_project_id = managedProjectId;
  await maybeStore(creds);
  return managedProjectId;
}

async function run() {
  const prompt = process.argv.slice(2).join(" ") || "Hello from the test script";
  const creds = await loadCreds();
  const accessToken = await ensureAccessToken(creds);
  const projectId = await ensureProjectBinding(creds, accessToken);

  const payload = wrapRequest(prompt, projectId);

  const resp = await fetch(`${CODE_ASSIST_ENDPOINT}/v1internal:generateContent`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${accessToken}`,
      "Content-Type": "application/json",
      ...CODE_ASSIST_HEADERS,
    },
    body: JSON.stringify(payload),
  });

  const text = await resp.text();
  console.log(`[gemini] Status: ${resp.status}`);
  console.log(text);
}

run().catch((error) => {
  console.error("[gemini] Request failed:", error);
  process.exitCode = 1;
});
