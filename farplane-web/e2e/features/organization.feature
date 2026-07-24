Feature: Active organization
  The SPA shows the active organization in the sidebar.
  Multi-org switch UI is not available yet; this locks the current org context.

  Scenario: Sidebar shows the active organization after sign-in
    Given authenticated e2e credentials are configured
    And the API is reachable
    When I sign in with E2E credentials
    Then I see the home page
    And the sidebar shows an organization name
