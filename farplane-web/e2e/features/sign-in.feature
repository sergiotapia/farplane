Feature: Sign in
  Critical path for email/password sign-in on a configured Farplane install.
  Global setup completes first-time install when needs_setup is true.

  Background:
    Given the API is reachable

  Scenario: Login page is available
    Given I open the login page
    Then I see the sign-in heading
    And I see email and password fields

  Scenario: Wrong password is rejected
    Given I open the login page
    When I sign in with "nobody@example.com" and "definitely-wrong-password"
    Then I see a sign-in alert
