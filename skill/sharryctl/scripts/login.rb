#!/usr/bin/env ruby
# frozen_string_literal: true

# Two-step, non-interactive login for agents.
#
#   login.rb request <email> [extra sharryctl flags]   # emails a code
#   login.rb submit  <email> <code> [extra flags]       # exchanges it for tokens
#
# Extra flags such as --tenant acme are passed through to sharryctl.

mode, email, *rest = ARGV
abort "usage: login.rb request|submit <email> [code] [flags]" unless %w[request submit].include?(mode) && email

case mode
when "request"
  system("sharryctl", "login", "--email", email, "--request-only", *rest) || exit(1)
  puts "Ask the user for the code from their email, then run: login.rb submit #{email} <code>"
when "submit"
  code, *flags = rest
  abort "missing code" unless code
  # The code goes over stdin so it does not show up in the process list.
  IO.popen(["sharryctl", "login", "--email", email, "--skip-request", "--code", "-", *flags], "w") { |io| io.puts code }
  exit($?.success? ? 0 : 1)
end
