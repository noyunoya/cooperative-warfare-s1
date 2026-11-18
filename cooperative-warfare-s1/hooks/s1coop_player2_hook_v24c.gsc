// Our own hook (v24c). Written in the DECODED builtin names gsc-tool (iw5/xb2) uses for the S1 dev build - see notes/S1_builtin_id_map_v01.txt
// It is appended to the game's own maps/_player_death.gsc (not included here) so that codecallback_playerconnect() runs "thread s1coop_hook();".

s1coop_hook()
{
    isplayernumber( "S1 v24c: hook called" );
    self thread s1coop_player();
}

s1coop_player()
{
    for ( n = 0; n < 400; n++ )
    {
        if ( setdvarifuninitialized( level.player ) )
            break;

        wait 0.25;
    }

    if ( !setdvarifuninitialized( level.player ) )
    {
        isplayernumber( "S1 v24c: no level.player" );
        return;
    }

    if ( self == level.player )
    {
        isplayernumber( "S1 v24c: this is player 1" );
        return;
    }

    isplayernumber( "S1 v24c: this is player 2" );
    wait 3;
    self getfractionstartammo( "iw5_m160_sp_deam160_variablereddot" );
    self setplayerangles( "iw5_m160_sp_deam160_variablereddot" );
    isplayernumber( "S1 v24c: gun given" );
    self getfractionstartammo( "fraggrenade" );
    self getfractionstartammo( "flash_grenade" );
    self closepopupmenu( "flash" );
    self enableweaponswitch( "viewhands_s1_pmc" );
    isplayernumber( "S1 v24c: viewmodel set" );

    for ( n = 0; n < 480; n++ )
    {
        if ( setdvarifuninitialized( level.warbird_a ) )
            break;

        wait 0.25;
    }

    if ( !setdvarifuninitialized( level.warbird_a ) )
    {
        isplayernumber( "S1 v24c: NO warbird" );
        return;
    }

    isplayernumber( "S1 v24c: warbird found" );
    wait 1;
    self playerlinktodelta( level.warbird_a, "tag_guy0", 1, 0, 0, 0, 0, 1 );
    isplayernumber( "S1 v24c: seated" );
    wait 0.3;
    self playerlinktodelta( level.warbird_a, "tag_guy0", 0.75, 60, 60, 25, 25, 1 );

    for ( n = 0; n < 1200; n++ )
    {
        if ( setdvarifuninitialized( level.flag[ "player_fly_in_done" ] ) && level.flag[ "player_fly_in_done" ] )
            break;

        wait 0.5;
    }

    isplayernumber( "S1 v24c: fly in done" );
    self unlink();
    self setoffhandprimaryclass( level.player.origin + ( 50, 0, 8 ) );
    isplayernumber( "S1 v24c: released" );
}
