function __ians5_has_subcommand
  set cmd (commandline -opc)
  if [ (count $cmd) -gt 1 ]
    return 0
  end
  return 1
end

function __ians5_lacks_command
  set cmd (commandline -o)
  if [ (count $cmd) -gt 1 -a -z "$cmd[2]" ]
    return 1
  end
  return 0

end

function __ians5_add_action_subcommand
    complete -xc proj -n '__ians5_lacks_command' -a $argv[1]
    complete -xc proj -n "__ians5_has_subcommand $argv[1]" -s r -l repo -a '(proj repo list | cut -f1 -d" ")'
    complete -xc proj -n "__ians5_has_subcommand $argv[1]" -a '(proj list)'
end

complete -xc proj -s h -l help -d 'help for proj'
complete -xc proj -s d -l debug -d 'turn on debug logging'

__ians5_add_action_subcommand create
__ians5_add_action_subcommand remove
__ians5_add_action_subcommand visit
__ians5_add_action_subcommand upload
__ians5_add_action_subcommand download

complete -xc proj -n "__ians5_has_subcommand upload" -s s -l service -a 'dropbox'
complete -xc proj -n "__ians5_has_subcommand download" -s s -l service -a 'dropbox'



complete -xc proj -n '__ians5_lacks_command' -alist
