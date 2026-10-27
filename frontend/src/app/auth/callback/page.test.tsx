import { render, screen, waitFor } from "@testing-library/react";
import { api } from "@/lib/api";
import GoogleCallbackPage from "./page";

const mockReplace = jest.fn();

jest.mock("@/lib/api", () => ({
  api: {
    post: jest.fn(),
  },
}));

jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace, refresh: jest.fn() }),
}));

describe("GoogleCallbackPage", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.mocked(api.post).mockResolvedValue({});
    sessionStorage.clear();
    window.history.replaceState({}, "", "/auth/callback");
  });

  it("exchanges the authorization code when the OAuth state matches", async () => {
    const state = "expected-state";
    sessionStorage.setItem("google_oauth_state", state);
    window.history.replaceState(
      {},
      "",
      `/auth/callback?code=authorization-code&state=${state}`
    );

    render(<GoogleCallbackPage />);

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith("/api/auth/google", {
        code: "authorization-code",
      });
    });
  });

  it("does not exchange a code when the OAuth state does not match", async () => {
    sessionStorage.setItem("google_oauth_state", "expected-state");
    window.history.replaceState(
      {},
      "",
      "/auth/callback?code=authorization-code&state=unexpected-state"
    );

    render(<GoogleCallbackPage />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Google sign-in could not be verified"
    );
    expect(api.post).not.toHaveBeenCalled();
  });
});
