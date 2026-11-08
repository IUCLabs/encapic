# bash completion for encapic
_encapic() {
	local cur=${COMP_WORDS[COMP_CWORD]} words=""
	case $COMP_CWORD in
	1) words="init set-testing unset-testing completion help --help --version" ;;
	2) [[ ${COMP_WORDS[1]} == completion ]] && words="bash zsh" ;;
	3) [[ ${COMP_WORDS[1]} == completion ]] && words="--install" ;;
	esac
	mapfile -t COMPREPLY < <(compgen -W "$words" -- "$cur")
}
complete -F _encapic encapic
