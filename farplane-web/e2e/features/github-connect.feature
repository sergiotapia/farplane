Feature: GitHub connect
  Owners and admins manage the Farplane GitHub App from Settings.

  Scenario: GitHub settings page is reachable after sign-in
    Given authenticated e2e credentials are configured
    And the API is reachable
    When I sign in with E2E credentials
    And I open GitHub settings
    Then I see the GitHub settings heading
    And I see GitHub connect actions or configuration guidance
